// actions_shared.js — Helpers shared by actions.js.
// Extracted so actions.js stays under the 150-line ceiling (CLAUDE.md §10
// rule #3) once the CR2 "Clear backups" handler is added.

// buildApplyWarning returns the standard warning text shown in apply
// confirmation dialogs. Reminds the user to close tools like Lightroom or
// Bridge that may hold the DNG file open — on Windows an open file handle
// will cause the GPS write to fail mid-flight, and the lazy-backup undo
// depends on a clean pre-apply state.
export function buildApplyWarning(photoCount) {
    const plural = photoCount !== 1 ? 's' : '';
    return `Apply GPS data to ${photoCount} photo${plural}?

Please close any app (Lightroom, Photoshop, Bridge, Explorer preview)
that may have these photos open — they must not be locked by another
process, or the write will fail.`;
}

// showToast displays a transient notification at the bottom-right of the screen.
export function showToast(msg, type = 'success') {
    let container = document.getElementById('toast-container');
    if (!container) {
        container = document.createElement('div');
        container.id = 'toast-container';
        document.body.appendChild(container);
    }
    const toast = document.createElement('div');
    toast.className = `toast toast-${type}`;
    toast.textContent = msg;
    container.appendChild(toast);
    setTimeout(() => {
        toast.classList.add('removing');
        toast.addEventListener('animationend', () => toast.remove(), { once: true });
    }, 3000);
}

// showConfirm shows a modal confirmation dialog and resolves to true (OK) or false (Cancel).
// The message is split on blank lines and each block is set via textContent on a <p>
// element — safe for multi-line warnings and XSS-safe by construction.
export function showConfirm(message) {
    return new Promise(resolve => {
        const overlay = document.createElement('div');
        overlay.className = 'confirm-overlay';

        const box = document.createElement('div');
        box.className = 'confirm-box';

        // Split on one or more blank lines — each paragraph becomes its own <p>.
        const paragraphs = String(message).split(/\n\s*\n/);
        paragraphs.forEach(block => {
            const p = document.createElement('p');
            p.style.whiteSpace = 'pre-line';
            p.textContent = block.trim();
            box.appendChild(p);
        });

        const btnRow = document.createElement('div');
        btnRow.className = 'btn-row';
        const okBtn = document.createElement('button');
        okBtn.className = 'btn btn-primary';
        okBtn.id = 'confirm-ok';
        okBtn.textContent = 'Confirm';
        const cancelBtn = document.createElement('button');
        cancelBtn.className = 'btn btn-secondary';
        cancelBtn.id = 'confirm-cancel';
        cancelBtn.textContent = 'Cancel';
        btnRow.appendChild(okBtn);
        btnRow.appendChild(cancelBtn);
        box.appendChild(btnRow);
        overlay.appendChild(box);

        document.body.appendChild(overlay);
        okBtn.addEventListener('click', () => { overlay.remove(); resolve(true); });
        cancelBtn.addEventListener('click', () => { overlay.remove(); resolve(false); });
    });
}
