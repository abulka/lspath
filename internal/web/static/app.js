const state = {
    data: null,
    selectedIndex: 0,
    filteredIndices: []
};

document.addEventListener('DOMContentLoaded', () => {
    fetchTrace();
    setupInput();
});

async function fetchTrace() {
    try {
        const response = await fetch('/api/trace');
        const data = await response.json();
        state.data = data;
        state.filteredIndices = data.PathEntries.map((_, i) => i);
        renderFlow();
        renderList();
        renderDetails();
    } catch (e) {
        document.getElementById('path-list').textContent = 'Error loading trace: ' + e;
    }
}

function setupInput() {
    document.addEventListener('keydown', (e) => {
        if (state.filteredIndices.length === 0) return;
        
        if (e.key === 'ArrowDown') {
            e.preventDefault();
            state.selectedIndex = Math.min(state.selectedIndex + 1, state.filteredIndices.length - 1);
            renderList();
            renderDetails();
            ensureVisible();
        } else if (e.key === 'ArrowUp') {
            e.preventDefault();
            state.selectedIndex = Math.max(state.selectedIndex - 1, 0);
            renderList();
            renderDetails();
            ensureVisible();
        }
    });
}

function renderList() {
    const list = document.getElementById('path-list');
    list.innerHTML = '';
    
    state.filteredIndices.forEach((dataIdx, viewIdx) => {
        const entry = state.data.PathEntries[dataIdx];
        const div = document.createElement('div');
        div.className = 'path-entry' + (viewIdx === state.selectedIndex ? ' selected' : '');
        div.textContent = entry.Value;
        div.onclick = () => {
            state.selectedIndex = viewIdx;
            renderList();
            renderDetails();
        };
        list.appendChild(div);
    });
}

function ensureVisible() {
    // Basic scroll into view logic
    const list = document.getElementById('path-list');
    const selected = list.children[state.selectedIndex];
    if (selected) {
        selected.scrollIntoView({ block: 'nearest' });
    }
}

function renderDetails() {
    const container = document.getElementById('details-content');
    if (state.filteredIndices.length === 0) {
        container.innerHTML = 'No selections';
        return;
    }
    
    const dataIdx = state.filteredIndices[state.selectedIndex];
    const entry = state.data.PathEntries[dataIdx];
    
    let html = `
        <div class="detail-row">
            <div class="detail-label">Directory</div>
            <div class="detail-value">${entry.Value}</div>
        </div>
        <div class="detail-row">
            <div class="detail-label">Source File</div>
            <div class="detail-value">${entry.SourceFile}:${entry.LineNumber}</div>
        </div>
        <div class="detail-row">
            <div class="detail-label">Mode</div>
            <div class="detail-value">${entry.Mode}</div>
        </div>
         <div class="detail-row">
            <div class="detail-label">Flow Node</div>
            <div class="detail-value">${entry.FlowID}</div>
        </div>
    `;
    
    if (entry.IsDuplicate) {
        html += `<div class="warning">⚠️ DUPLICATE<br><br>${entry.Remediation}</div>`;
    } else {
        html += `<div class="success" style="margin-top:20px">✅ Valid Entry</div>`;
    }
    
    container.innerHTML = html;
}

function renderFlow() {
    const bar = document.getElementById('flow-bar');
    if (!state.data.FlowNodes) return;
    
    state.data.FlowNodes.forEach(node => {
        const div = document.createElement('div');
        div.className = 'flow-node';
        div.textContent = node.FilePath.split('/').pop(); // basename
        div.title = node.FilePath;
        bar.appendChild(div);
    });
}
