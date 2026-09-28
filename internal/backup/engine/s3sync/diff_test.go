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

import "testing"

const emptyMD5 = "d41d8cd98f00b204e9800998ecf8427e"

func TestComputeDiff(t *testing.T) {
	tests := []struct {
		name        string
		local       []localFile
		remote      []remoteObject
		wantPut     []string // keys
		wantDelete  []string
		wantSkipped int64
	}{
		{
			name:       "empty local, non-empty remote deletes everything",
			local:      nil,
			remote:     []remoteObject{{Key: "a/1.txt", Size: 10, ETag: "abc"}},
			wantDelete: []string{"a/1.txt"},
		},
		{
			name:    "non-empty local, empty remote puts everything",
			local:   []localFile{{Key: "a/1.txt", Size: 10, MD5: "x"}},
			remote:  nil,
			wantPut: []string{"a/1.txt"},
		},
		{
			name:    "zero-byte new file must still be put",
			local:   []localFile{{Key: "a/empty.txt", Size: 0, MD5: emptyMD5}},
			remote:  nil,
			wantPut: []string{"a/empty.txt"},
		},
		{
			name:        "zero-byte file matching remote zero-byte object is skipped",
			local:       []localFile{{Key: "a/empty.txt", Size: 0, MD5: emptyMD5}},
			remote:      []remoteObject{{Key: "a/empty.txt", Size: 0, ETag: emptyMD5}},
			wantSkipped: 0,
		},
		{
			name:        "matching size and MD5 is skipped",
			local:       []localFile{{Key: "a/1.txt", Size: 5, MD5: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
			remote:      []remoteObject{{Key: "a/1.txt", Size: 5, ETag: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
			wantSkipped: 5,
		},
		{
			name:    "size mismatch is put even if ETag would match",
			local:   []localFile{{Key: "a/1.txt", Size: 6, MD5: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
			remote:  []remoteObject{{Key: "a/1.txt", Size: 5, ETag: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
			wantPut: []string{"a/1.txt"},
		},
		{
			name:    "MD5 mismatch with same size is put",
			local:   []localFile{{Key: "a/1.txt", Size: 5, MD5: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
			remote:  []remoteObject{{Key: "a/1.txt", Size: 5, ETag: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}},
			wantPut: []string{"a/1.txt"},
		},
		{
			name:    "composite (multipart) ETag is always put even if size matches",
			local:   []localFile{{Key: "a/big.bin", Size: 10 << 20, MD5: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
			remote:  []remoteObject{{Key: "a/big.bin", Size: 10 << 20, ETag: "deadbeefdeadbeefdeadbeefdeadbeef-3"}},
			wantPut: []string{"a/big.bin"},
		},
		{
			name: "put and delete sets are disjoint",
			local: []localFile{
				{Key: "a/keep.txt", Size: 1, MD5: "cccccccccccccccccccccccccccccccc"},
				{Key: "a/new.txt", Size: 2, MD5: "dddddddddddddddddddddddddddddddd"},
			},
			remote: []remoteObject{
				{Key: "a/keep.txt", Size: 1, ETag: "cccccccccccccccccccccccccccccccc"},
				{Key: "a/gone.txt", Size: 3, ETag: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"},
			},
			wantPut:     []string{"a/new.txt"},
			wantDelete:  []string{"a/gone.txt"},
			wantSkipped: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeDiff(tt.local, tt.remote)

			gotPut := keysOf(got.ToPut)
			if !sameSet(gotPut, tt.wantPut) {
				t.Errorf("ToPut = %v, want %v", gotPut, tt.wantPut)
			}
			if !sameSet(got.ToDelete, tt.wantDelete) {
				t.Errorf("ToDelete = %v, want %v", got.ToDelete, tt.wantDelete)
			}
			if got.Skipped != tt.wantSkipped {
				t.Errorf("Skipped = %d, want %d", got.Skipped, tt.wantSkipped)
			}

			for _, k := range gotPut {
				for _, d := range got.ToDelete {
					if k == d {
						t.Errorf("key %q present in both ToPut and ToDelete", k)
					}
				}
			}
		})
	}
}

func keysOf(files []localFile) []string {
	keys := make([]string, len(files))
	for i, f := range files {
		keys[i] = f.Key
	}
	return keys
}

func sameSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := make(map[string]bool, len(want))
	for _, w := range want {
		seen[w] = true
	}
	for _, g := range got {
		if !seen[g] {
			return false
		}
	}
	return true
}
