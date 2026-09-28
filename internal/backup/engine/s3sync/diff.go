// MIT License
//
// Copyright (c) 2026 PlusClouds
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.
package s3sync

import "strings"

// localFile describes one file under a source root, keyed by its
// bucket-relative object key (see objectKey in snapshot.go).
type localFile struct {
	Key  string // {basename(sourceRoot)}/{relative path}
	Path string // absolute local path, for opening during PUT
	Size int64
	MD5  string // lowercase hex, always computed by walkLocal
}

// remoteObject is the subset of minio.ObjectInfo the diff needs.
type remoteObject struct {
	Key  string
	Size int64
	ETag string
}

// diffResult is what to do to make the bucket match the local tree exactly.
type diffResult struct {
	ToPut    []localFile
	ToDelete []string
	Skipped  int64 // bytes skipped because already up to date -> SnapshotResult.BytesDeduped
}

// isPlainMD5ETag reports whether etag looks like a plain single-part-upload
// MD5 (32 lowercase hex chars) rather than a multipart/composite ETag (which
// has a "-N" part-count suffix and is not a hash of the object's bytes at
// all — S3 does not provide a cheap, trustworthy content hash for multipart
// uploads).
func isPlainMD5ETag(etag string) bool {
	if len(etag) != 32 {
		return false
	}
	for _, c := range etag {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

// computeDiff decides what to PUT and DELETE to make the bucket match local
// exactly. Rule, per file:
//   - no remote object with the same key -> PUT (new)
//   - remote size differs -> PUT (changed)
//   - remote ETag is composite (multipart upload) -> always PUT, even if size
//     matches: a multipart ETag isn't a hash of the object's bytes, so a
//     same-size content change can't be cheaply detected — trusting it would
//     be a silent correctness gap, not just a missed optimization.
//   - remote ETag is a plain MD5 and matches local.MD5 -> skip (up to date)
//   - remote ETag is a plain MD5 and differs -> PUT (changed)
//
// Any remote key with no matching local file is deleted (mirror semantics).
func computeDiff(local []localFile, remote []remoteObject) diffResult {
	remoteByKey := make(map[string]remoteObject, len(remote))
	for _, r := range remote {
		remoteByKey[r.Key] = r
	}

	localKeys := make(map[string]struct{}, len(local))
	var result diffResult

	for _, lf := range local {
		localKeys[lf.Key] = struct{}{}

		r, ok := remoteByKey[lf.Key]
		switch {
		case !ok:
			result.ToPut = append(result.ToPut, lf)
		case r.Size != lf.Size:
			result.ToPut = append(result.ToPut, lf)
		case !isPlainMD5ETag(r.ETag):
			result.ToPut = append(result.ToPut, lf)
		case !strings.EqualFold(r.ETag, lf.MD5):
			result.ToPut = append(result.ToPut, lf)
		default:
			result.Skipped += lf.Size
		}
	}

	for _, r := range remote {
		if _, ok := localKeys[r.Key]; !ok {
			result.ToDelete = append(result.ToDelete, r.Key)
		}
	}

	return result
}
