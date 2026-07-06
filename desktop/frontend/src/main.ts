import './style.css';
import './app.css';
import {
    GetStatus,
    Register,
    PauseJob,
    ResumeJob,
    RunJobNow,
    CancelJob,
    SetAutoStart,
    GetAutoStart,
} from '../wailsjs/go/main/App';
import { EventsOn, ClipboardSetText, BrowserOpenURL } from '../wailsjs/runtime/runtime';
import stashLogo from './assets/images/stash-logo.png';

const app = document.querySelector<HTMLDivElement>('#app')!;

type JobView = {
    uuid: string;
    name: string;
    schedule: string;
    paused: boolean;
    lastRunStatus: string;
    lastRunAt: string;
    running: boolean;
};

type StatusView = {
    registered: boolean;
    agentUuid: string;
    bucket: string;
    syncEngine: string;
    daemonUp: boolean;
    daemonErr?: string;
    jobs: JobView[];
};

let pollHandle: number | undefined;
let lastError = '';

// currentView guards against the 3s poll loop wiping out in-progress input:
// GetStatus() keeps reporting "not registered" while the user is filling in
// the token/endpoint fields, and re-rendering renderRegister()'s innerHTML on
// every tick would reset those inputs to empty mid-keystroke. Only the first
// transition into a view actually re-renders it; the dashboard has no free
// text inputs, so it's fine to keep refreshing every tick once registered.
let currentView: 'register' | 'dashboard' | null = null;

function escapeHtml(s: string): string {
    const div = document.createElement('div');
    div.textContent = s;
    return div.innerHTML;
}

// copyButton renders a small "Copy" button wired via event delegation (see
// wireCopyButtons) — used next to values the user shouldn't have to
// manually select (fiddly for a 40+ char access/secret key).
function copyButton(value: string): string {
    return `<button class="copy-btn" data-copy="${escapeHtml(value)}" title="Copy to clipboard">Copy</button>`;
}

function wireCopyButtons(root: ParentNode) {
    root.querySelectorAll('button[data-copy]').forEach((btn) => {
        btn.addEventListener('click', async () => {
            const value = (btn as HTMLElement).dataset.copy!;
            try {
                await ClipboardSetText(value);
                const original = btn.textContent;
                btn.textContent = 'Copied!';
                setTimeout(() => {
                    btn.textContent = original;
                }, 1200);
            } catch (err) {
                alert('Could not copy: ' + String(err));
            }
        });
    });
}

async function refresh() {
    let status: StatusView | null = null;
    try {
        status = (await GetStatus()) as unknown as StatusView;
        lastError = '';
    } catch (e) {
        // GetStatus() itself failed (e.g. backend unreachable) — that's a
        // real "fall back to register view" case.
        lastError = String(e);
        render(null);
        return;
    }
    // A bug in rendering (as opposed to GetStatus() failing) must not fall
    // back to the register view — doing so previously wiped an in-progress
    // registration's state on an unrelated rendering crash. Surface it
    // instead of hiding it behind a form reset.
    try {
        render(status);
    } catch (e) {
        console.error('render() failed:', e);
    }
}

function render(status: StatusView | null) {
    if (!status || !status.registered) {
        if (currentView === 'register') return;
        currentView = 'register';
        renderRegister();
        return;
    }
    currentView = 'dashboard';
    renderDashboard(status);
}

function topbar(): string {
    return `
    <div class="topbar">
      <img class="logo" src="${stashLogo}" alt="Stash" />
    </div>
  `;
}

function renderRegister() {
    app.innerHTML = `
    ${topbar()}
    <div class="page">
      <div class="card">
        <h1>Connect this machine</h1>
        <p class="muted">Paste the one-time registration token from your PlusClouds dashboard.</p>
        ${lastError ? `<p class="error">${escapeHtml(lastError)}</p>` : ''}
        <label>Registration token
          <input id="token" type="text" placeholder="token" />
        </label>
        <label>Platform endpoint
          <input id="endpoint" type="text" placeholder="https://api.example.com" />
        </label>
        <button id="register-btn" class="btn-primary">Register</button>
        <div id="register-status"></div>
      </div>
    </div>
  `;

    const registerBtn = document.getElementById('register-btn') as HTMLButtonElement;
    registerBtn.addEventListener('click', async () => {
        const tokenInput = document.getElementById('token') as HTMLInputElement;
        const endpointInput = document.getElementById('endpoint') as HTMLInputElement;
        const token = tokenInput.value.trim();
        const endpoint = endpointInput.value.trim();
        const statusEl = document.getElementById('register-status')!;
        if (!token || !endpoint) {
            statusEl.innerHTML = `<p class="error">Token and endpoint are both required.</p>`;
            return;
        }
        registerBtn.disabled = true;
        tokenInput.disabled = true;
        endpointInput.disabled = true;
        statusEl.innerHTML = `<div class="spinner-row"><span class="spinner"></span> Registering…</div>`;
        try {
            await Register(token, endpoint);
            // No secrets shown here — the platform already has this agent's
            // credentials; move straight to the dashboard once GetStatus()
            // reports registered.
            currentView = null; // force renderDashboard on the next refresh
            await refresh();
        } catch (e) {
            registerBtn.disabled = false;
            tokenInput.disabled = false;
            endpointInput.disabled = false;
            statusEl.innerHTML = `<p class="error">Registration failed: ${escapeHtml(String(e))}</p>`;
        }
    });
}

function stateBadge(j: JobView): string {
    if (j.running) return `<span class="badge green">running</span>`;
    if (j.paused) return `<span class="badge gray">paused</span>`;
    return `<span class="badge gray">idle</span>`;
}

function renderDashboard(status: StatusView) {
    const rows = (status.jobs || [])
        .map(
            (j) => `
      <tr>
        <td>${escapeHtml(j.name)}</td>
        <td>${escapeHtml(j.schedule)}</td>
        <td>${stateBadge(j)}</td>
        <td>${escapeHtml(j.lastRunStatus || '-')}</td>
        <td>${escapeHtml(j.lastRunAt || '-')}</td>
        <td class="actions">
          <label class="toggle" title="${j.paused ? 'Resume' : 'Pause'}">
            <input type="checkbox" data-action="toggle-pause" data-uuid="${j.uuid}" ${j.paused ? '' : 'checked'} />
            <span class="slider"></span>
          </label>
          <button class="btn-link" data-action="run" data-uuid="${j.uuid}" ${j.running ? 'disabled' : ''}>Run Now</button>
          <button class="btn-link danger" data-action="cancel" data-uuid="${j.uuid}" ${j.running ? '' : 'disabled'}>Cancel</button>
        </td>
      </tr>
    `
        )
        .join('');

    const daemonBadge = status.daemonUp
        ? `<span class="badge green">active</span>`
        : status.daemonErr
            ? `<span class="badge red">error</span>`
            : `<span class="badge gray">starting</span>`;

    app.innerHTML = `
    ${topbar()}
    <div class="page">
      <div class="card">
        <div class="card-header">
          <div>
            <h1>This machine</h1>
            <div class="badges">
              ${daemonBadge}
              <span class="badge gray">${escapeHtml(status.syncEngine)}</span>
            </div>
          </div>
        </div>

        <div class="meta-grid">
          <div class="meta-full">
            <span class="meta-label">Agent</span>
            <span class="meta-value"><span class="value-text">${escapeHtml(status.agentUuid)}</span>${copyButton(status.agentUuid)}</span>
          </div>
          <div>
            <span class="meta-label">Bucket</span>
            <span class="meta-value">${escapeHtml(status.bucket)}</span>
          </div>
          <div>
            <span class="meta-label">Engine</span>
            <span class="meta-value">${escapeHtml(status.syncEngine)}</span>
          </div>
          <div>
            <span class="meta-label">Daemon</span>
            <span class="meta-value">${status.daemonUp ? 'Running' : (status.daemonErr ? 'Error: ' + escapeHtml(status.daemonErr) : 'Starting…')}</span>
          </div>
        </div>

        <label class="autostart-row">
          <span class="toggle">
            <input type="checkbox" id="autostart-checkbox" />
            <span class="slider"></span>
          </span>
          Start at login
        </label>
      </div>

      <div class="card">
        <div class="section-header">
          <div class="section-title">Jobs</div>
          <button class="btn-secondary" id="manage-jobs-link">Manage Backup Jobs</button>
        </div>
        <table class="jobs">
          <thead>
            <tr><th>Name</th><th>Schedule</th><th>State</th><th>Last run</th><th>At</th><th></th></tr>
          </thead>
          <tbody>${rows || `<tr><td colspan="6" class="muted">No jobs yet — <button class="btn-link" id="add-job-link">add one on the platform</button></td></tr>`}</tbody>
        </table>
      </div>
    </div>
  `;

    wireCopyButtons(app);

    const openJobsPage = () => BrowserOpenURL(`https://cloud.plusclouds.com/storage/s3/backup-agents/${status.agentUuid}`);
    document.getElementById('manage-jobs-link')?.addEventListener('click', openJobsPage);
    document.getElementById('add-job-link')?.addEventListener('click', openJobsPage);

    GetAutoStart().then((enabled: boolean) => {
        (document.getElementById('autostart-checkbox') as HTMLInputElement).checked = enabled;
    });
    document.getElementById('autostart-checkbox')!.addEventListener('change', async (e) => {
        const checked = (e.target as HTMLInputElement).checked;
        try {
            await SetAutoStart(checked);
        } catch (err) {
            alert('Could not update auto-start: ' + String(err));
        }
    });

    app.querySelectorAll('[data-action]').forEach((el) => {
        el.addEventListener(el.tagName === 'INPUT' ? 'change' : 'click', async () => {
            const uuid = (el as HTMLElement).dataset.uuid!;
            const action = (el as HTMLElement).dataset.action!;
            try {
                if (action === 'toggle-pause') {
                    const checked = (el as HTMLInputElement).checked;
                    if (checked) await ResumeJob(uuid);
                    else await PauseJob(uuid);
                } else if (action === 'run') await RunJobNow(uuid);
                else if (action === 'cancel') await CancelJob(uuid);
                await refresh();
            } catch (err) {
                alert('Action failed: ' + String(err));
            }
        });
    });
}

// Primary update mechanism: the Go side (runner.Runner's OnStart/OnComplete
// hooks, wired in desktop/app.go) emits "stash:changed" whenever a job
// starts/finishes — cron-triggered or run_job_now, from either this GUI or
// the platform — so the dashboard updates immediately without polling.
EventsOn('stash:changed', refresh);

// Safety-net poll, deliberately infrequent: covers the rare case of a missed
// event (e.g. a race during app startup) without reintroducing constant
// polling — 30s, not 3s, since events now carry the real-time load.
refresh();
pollHandle = window.setInterval(refresh, 30000);
window.addEventListener('beforeunload', () => {
    if (pollHandle) window.clearInterval(pollHandle);
});
