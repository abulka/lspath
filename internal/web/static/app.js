const state = {
    data: null,
    currentView: 'main',
    // Main View State
    mainSelectedIndex: 0,
    mainFilteredIndices: [],
    searchTerm: '',
    currentLs: null,
    // Flow View State
    flowNodeIndex: 0,
    cumulative: true,
    flowSelectedIndex: 0,
    fileCache: {},
    flowInitialized: false,
    // Diagnostics State
    verbose: false,
    // Which Mode state
    whichMatches: []
};

document.addEventListener('DOMContentLoaded', () => {
    fetchTrace();
    setupInputs();
    setupSplitter('flow-splitter', 'flow-list-panel');
    setupSplitter('explorer-splitter', '#main-panel > .list-panel');
});

async function fetchTrace() {
    try {
        const response = await fetch('/api/trace');
        state.data = await response.json();

        state.mainFilteredIndices = state.data.PathEntries.map((_, i) => i);

        document.getElementById('version-display').textContent = 'v' + state.data.Version;
        updateDiagnostics();

        renderAll();
    } catch (e) {
        console.error(e);
        const list = document.getElementById('path-list');
        if (list) list.textContent = 'Error loading trace: ' + e;
    }
}

function setupInputs() {
    // Search input
    const searchInput = document.getElementById('search-input');
    searchInput.addEventListener('keydown', (e) => {
        if (e.key === 'Enter') {
            performWhichSearch();
        }
    });
    // Clear search on empty input
    searchInput.addEventListener('input', (e) => {
        if (e.target.value === '') {
            state.mainFilteredIndices = state.data.PathEntries.map((_, i) => i);
            state.whichMatches = [];
            state.searchTerm = '';
            renderAll();
        }
    });

    // Keyboard navigation
    document.addEventListener('keydown', (e) => {
        if (state.currentView === 'diagnostics') return;

        if (e.key === 'ArrowDown' || e.key === 'j') {
            e.preventDefault();
            moveSelection(1);
        } else if (e.key === 'ArrowUp' || e.key === 'k') {
            e.preventDefault();
            moveSelection(-1);
        } else if (e.key === 'Escape') {
            clearSearch();
        } else if (state.currentView === 'flow') {
            if (e.key === 'ArrowLeft' || e.key === 'h') {
                e.preventDefault();
                moveFlowNode(-1);
            } else if (e.key === 'ArrowRight' || e.key === 'l') {
                e.preventDefault();
                moveFlowNode(1);
            }
        }
    });
}

function clearSearch() {
    state.mainFilteredIndices = state.data.PathEntries.map((_, i) => i);
    state.whichMatches = [];
    state.searchTerm = '';
    const input = document.getElementById('search-input');
    if (input) input.value = '';
    const clearBtn = document.getElementById('clear-search');
    if (clearBtn) clearBtn.style.display = 'none';
    renderAll();
}

function setupSplitter(splitterId, panelSelector) {
    const splitter = document.getElementById(splitterId);
    let panel;
    if (panelSelector.includes('>')) {
        panel = document.querySelector(panelSelector);
    } else {
        panel = document.getElementById(panelSelector);
    }

    let isDragging = false;

    if (!splitter || !panel) return;

    splitter.addEventListener('mousedown', (e) => {
        isDragging = true;
        splitter.classList.add('active');
        document.body.style.cursor = 'col-resize';
        document.body.style.userSelect = 'none';
    });

    document.addEventListener('mousemove', (e) => {
        if (!isDragging) return;

        const container = panel.parentElement;
        const containerRect = container.getBoundingClientRect();
        const offsetX = e.clientX - containerRect.left;

        const percentage = (offsetX / containerRect.width) * 100;
        if (percentage > 10 && percentage < 90) {
            panel.style.flex = `0 0 ${percentage}%`;
        }
    });

    document.addEventListener('mouseup', () => {
        if (isDragging) {
            isDragging = false;
            splitter.classList.remove('active');
            document.body.style.cursor = '';
            document.body.style.userSelect = '';
        }
    });
}

function moveFlowNode(delta) {
    if (!state.data) return;
    const newIdx = Math.max(0, Math.min(state.flowNodeIndex + delta, state.data.FlowNodes.length - 1));
    if (newIdx !== state.flowNodeIndex) {
        state.flowNodeIndex = newIdx;
        renderAll();
    }
}

function switchView(viewId) {
    state.currentView = viewId;

    // Update navigation UI
    document.querySelectorAll('.nav-item').forEach(el => el.classList.remove('active'));
    const navEl = document.getElementById('nav-' + viewId);
    if (navEl) navEl.classList.add('active');

    // Update View Panels
    document.querySelectorAll('.view-panel').forEach(el => el.classList.remove('active'));
    const panelEl = document.getElementById(viewId + '-panel');
    if (panelEl) panelEl.classList.add('active');

    // Logic for specific views
    if (viewId === 'flow' && !state.flowInitialized) {
        initializeFlowMode();
        updateCumulativeButton();
    }

    renderAll();
}

function initializeFlowMode() {
    if (!state.data) return;
    // Default to .zshrc or first node
    const zshrcIdx = state.data.FlowNodes.findIndex(n => n.FilePath.endsWith('.zshrc'));
    if (zshrcIdx !== -1) {
        state.flowNodeIndex = zshrcIdx;
    } else {
        state.flowNodeIndex = 0;
    }
    state.flowInitialized = true;
}

function renderAll() {
    if (!state.data) return;

    if (state.currentView === 'main') {
        renderPathList('path-list', state.mainFilteredIndices, state.mainSelectedIndex, 'main');
        renderDetails();
    } else if (state.currentView === 'flow') {
        renderFlowNodes();
        renderFlowPathList();
        renderFilePreview();
    }
}

function moveSelection(delta) {
    if (state.currentView === 'main') {
        if (state.mainFilteredIndices.length === 0) return;
        state.mainSelectedIndex = Math.max(0, Math.min(state.mainSelectedIndex + delta, state.mainFilteredIndices.length - 1));
        renderPathList('path-list', state.mainFilteredIndices, state.mainSelectedIndex, 'main');
        renderDetails();
    } else if (state.currentView === 'flow') {
        // Selection in flow mode list
        if (state.data.PathEntries.length === 0) return;
        state.flowSelectedIndex = Math.max(0, Math.min(state.flowSelectedIndex + delta, state.data.PathEntries.length - 1));

        // Bidirectional selection: select node that contributed this entry
        const entry = state.data.PathEntries[state.flowSelectedIndex];
        const nodeIdx = state.data.FlowNodes.findIndex(n => n.ID === entry.FlowID);
        if (nodeIdx !== -1) {
            state.flowNodeIndex = nodeIdx;
        }

        renderAll();
        // Ensure the selected path is visible
        const list = document.getElementById('flow-path-list');
        const selected = list.children[state.flowSelectedIndex];
        if (selected) selected.scrollIntoView({ block: 'nearest' });
    }
}

function renderPathList(containerId, indices, selectedIdx, viewType) {
    const list = document.getElementById(containerId);
    if (!list) return;
    list.innerHTML = '';

    indices.forEach((dataIdx, viewIdx) => {
        const entry = state.data.PathEntries[dataIdx];
        const div = document.createElement('div');
        div.className = 'item-row' + (viewIdx === selectedIdx ? ' selected' : '');

        if (viewType === 'flow') {
            const activeNode = state.data.FlowNodes[state.flowNodeIndex];
            let isHighlighted = false;

            if (state.cumulative) {
                const nodeOfEntry = state.data.FlowNodes.find(n => n.ID === entry.FlowID);
                if (nodeOfEntry && nodeOfEntry.Order <= activeNode.Order) {
                    isHighlighted = true;
                }
            } else {
                if (entry.FlowID === activeNode.ID) {
                    isHighlighted = true;
                }
            }

            if (isHighlighted) {
                div.classList.add('highlighted');
            } else {
                div.classList.add('dimmed');
            }
        }

        const nameSpan = document.createElement('span');
        let label = `${dataIdx + 1}. ${entry.Value}`;

        // Show matched binary if in search mode
        if (viewType === 'main' && state.whichMatches.length > 0) {
            const match = state.whichMatches.find(m => m.Index === dataIdx);
            if (match) {
                label = `${dataIdx + 1}. ${match.MatchedFile} (${entry.Value})`;
            }
        }

        nameSpan.textContent = label;
        div.appendChild(nameSpan);

        // Priority indicators
        if (dataIdx === 0) {
            const priority = document.createElement('span');
            priority.style.marginLeft = '10px';
            priority.style.fontSize = '0.8em';
            priority.style.color = 'var(--accent-bright)';
            priority.textContent = `(highest priority ${Icons.PriorityHigh})`;
            div.appendChild(priority);
        } else if (dataIdx === state.data.PathEntries.length - 1) {
            const priority = document.createElement('span');
            priority.style.marginLeft = '10px';
            priority.style.fontSize = '0.8em';
            priority.style.color = 'var(--text-muted)';
            priority.textContent = `(lowest priority ${Icons.PriorityLow})`;
            div.appendChild(priority);
        }

        if (entry.IsDuplicate) {
            const status = document.createElement('span');
            status.className = 'status-pill';
            status.textContent = `dup ${Icons.Duplicate}`;
            div.appendChild(status);
        } else if (entry.SymlinkPointsTo >= 0) {
            const status = document.createElement('span');
            status.className = 'status-pill';
            status.style.background = '#3b82f6';
            status.textContent = `symlink ${Icons.Duplicate}${Icons.Symlink}`;
            div.appendChild(status);
        } else if (entry.Diagnostics && entry.Diagnostics.some(d => d.includes('does not exist'))) {
            const status = document.createElement('span');
            status.className = 'status-pill';
            status.textContent = `missing ${Icons.Missing}`;
            div.appendChild(status);
        }

        div.onclick = () => {
            if (viewType === 'main') {
                state.mainSelectedIndex = viewIdx;
                renderDetails();
                renderPathList(containerId, indices, state.mainSelectedIndex, viewType);
            } else {
                state.flowSelectedIndex = viewIdx;
                // Bidirectional logic on click
                const nodeIdx = state.data.FlowNodes.findIndex(n => n.ID === entry.FlowID);
                if (nodeIdx !== -1) state.flowNodeIndex = nodeIdx;
                renderAll();
            }
        };
        list.appendChild(div);
    });
}

async function renderDetails() {
    const container = document.getElementById('details-content');
    const lsContainer = document.getElementById('directory-listing-container');
    if (!container) return;

    if (state.mainFilteredIndices.length === 0) {
        container.innerHTML = `
            <div class="detail-card">
                <p style="color:var(--text-muted); text-align:center; padding: 20px;">
                    No matches found for "${state.searchTerm}".
                </p>
            </div>
        `;
        if (lsContainer) lsContainer.innerHTML = '';
        return;
    }

    const dataIdx = state.mainFilteredIndices[state.mainSelectedIndex];
    if (dataIdx === undefined) return;
    const entry = state.data.PathEntries[dataIdx];
    const match = state.whichMatches.find(m => m.Index === dataIdx);

    let html = `
        <div class="detail-card">
            ${match ? `
            <div class="detail-row" style="background:rgba(122, 162, 247, 0.1); padding:10px; border-radius:6px; margin-bottom:20px;">
                <div class="detail-label" style="color:var(--accent-bright)">Matched Binary</div>
                <div class="detail-value" style="font-size:1.4em; color:var(--accent-bright); font-weight:bold;">${match.MatchedFile}</div>
                <div class="detail-label" style="margin-top:5px;">Full Path</div>
                <div class="detail-value" style="font-size:0.9em; opacity:0.8">${entry.Value}/${match.MatchedFile}</div>
            </div>
            ` : ''}
            <div class="detail-row">
                <div class="detail-label">Directory</div>
                <div class="detail-value">${entry.Value}</div>
            </div>
            <div class="detail-row">
                <div class="detail-label">Source</div>
                <div class="detail-value">${entry.SourceFile}:${entry.LineNumber}</div>
            </div>
            <div class="detail-row">
                <div class="detail-label">Shell Mode</div>
                <div class="detail-value">${entry.Mode}</div>
            </div>
    `;

    if (entry.IsDuplicate) {
        html += `
            <div class="alert alert-warning">
                <strong>${Icons.Duplicate} Duplicate detected</strong><br>
                ${entry.DuplicateMessage}
            </div>
        `;
    } else if (entry.SymlinkPointsTo >= 0) {
        html += `
            <div class="alert alert-warning" style="background:rgba(59,130,246,0.1);border-color:rgba(59,130,246,0.3);">
                <strong>🔗 SYMLINK ${Icons.Duplicate}${Icons.Symlink} DETECTED</strong><br>
                ${entry.SymlinkMessage}<br>
                <em style="color:var(--text-muted);font-size:0.9em;">This is normal on modern Linux systems.</em>
            </div>
        `;
    }

    html += `</div>`;
    container.innerHTML = html;

    // Fetch and render LS-like listing
    lsContainer.innerHTML = '<p style="color:var(--text-muted); padding:20px;">Loading directory listing...</p>';
    try {
        const resp = await fetch(`/api/ls?path=${encodeURIComponent(entry.Value)}`);
        if (!resp.ok) throw new Error("Could not load directory listing");
        const files = await resp.json();
        state.currentLs = files;
        renderLsTable(files);
    } catch (e) {
        lsContainer.innerHTML = `<p style="color:var(--warning); padding:20px;">Error: ${e.message}</p>`;
    }
}

function renderLsTable(files) {
    const container = document.getElementById('directory-listing-container');
    if (!container) return;

    let html = `
        <table class="listing-table">
            <thead>
                <tr>
                    <th>Mode</th>
                    <th>Size</th>
                    <th>Modified</th>
                    <th>Name</th>
                </tr>
            </thead>
            <tbody>
    `;

    files.forEach(f => {
        const sizeStr = formatSize(f.Size);
        const nameClass = f.IsDir ? 'listing-dir' : 'listing-name';
        html += `
            <tr>
                <td style="color:var(--text-muted)">${f.Mode}</td>
                <td style="text-align:right">${sizeStr}</td>
                <td>${f.ModTime}</td>
                <td class="${nameClass}">${f.IsDir ? '📁' : '📄'} ${f.Name}</td>
            </tr>
        `;
    });

    html += `</tbody></table>`;
    container.innerHTML = html;
}

function formatSize(bytes) {
    if (bytes === 0) return '0';
    const k = 1024;
    const sizes = ['', 'K', 'M', 'G'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    if (i === 0) return bytes.toString();
    return (bytes / Math.pow(k, i)).toFixed(1) + sizes[i];
}

function renderFlowNodes() {
    const container = document.getElementById('flow-nodes');
    if (!container) return;
    container.innerHTML = '';

    // Add Start Node
    const startNode = document.createElement('div');
    startNode.className = 'flow-node start-node';
    startNode.textContent = '🚀 Shell Start';
    container.appendChild(startNode);

    state.data.FlowNodes.forEach((node, idx) => {
        // Arrow separator
        const arrow = document.createElement('div');
        arrow.className = 'flow-arrow';
        arrow.textContent = '→';
        container.appendChild(arrow);

        const div = document.createElement('div');
        div.className = 'flow-node' + (idx === state.flowNodeIndex ? ' active' : '');
        
        let nodeText = node.FilePath.split('/').pop();
        if (idx === 0) nodeText += ' ' + Icons.First;
        if (idx === state.data.FlowNodes.length - 1) nodeText += ' ' + Icons.Last;
        
        div.textContent = nodeText;
        div.title = node.FilePath;
        div.onclick = () => {
            state.flowNodeIndex = idx;
            renderAll();
        };
        container.appendChild(div);
    });

    // Scroll active node into view
    const active = container.querySelector('.flow-node.active');
    if (active) active.scrollIntoView({ inline: 'center', behavior: 'smooth' });
}

function renderFlowPathList() {
    const allIndices = state.data.PathEntries.map((_, i) => i);
    renderPathList('flow-path-list', allIndices, state.flowSelectedIndex, 'flow');
}

async function renderFilePreview() {
    const node = state.data.FlowNodes[state.flowNodeIndex];
    if (!node) return;
    const preview = document.getElementById('file-preview');
    const filenameLabel = document.getElementById('preview-filename');

    filenameLabel.textContent = node.FilePath;

    if (node.NotExecuted) {
        preview.textContent = "(File not executed during this shell session)";
        return;
    }

    if (state.fileCache[node.FilePath]) {
        applyPreview(state.fileCache[node.FilePath]);
        return;
    }

    try {
        const resp = await fetch(`/api/file?path=${encodeURIComponent(node.FilePath)}`);
        if (!resp.ok) throw new Error("Could not load file");
        const text = await resp.text();
        state.fileCache[node.FilePath] = text;
        applyPreview(text);
    } catch (e) {
        preview.textContent = "Error loading file content: " + e.message;
    }
}

function applyPreview(text) {
    const preview = document.getElementById('file-preview');
    preview.innerHTML = '';

    const lines = text.split('\n');
    lines.forEach((line, i) => {
        const div = document.createElement('div');
        div.className = 'preview-line';

        const trimmed = line.trim();
        if (trimmed.includes('PATH=') || trimmed.includes('export PATH') ||
            trimmed.startsWith('source ') || trimmed.startsWith('. ')) {
            div.classList.add('highlight-line');
        }

        div.textContent = `${(i + 1).toString().padStart(3, ' ')} | ${line}`;
        preview.appendChild(div);
    });
}

function toggleCumulative() {
    state.cumulative = !state.cumulative;
    updateCumulativeButton();
    renderAll();
}

function updateCumulativeButton() {
    const btn = document.getElementById('toggle-cumulative');
    if (btn) {
        if (state.cumulative) btn.classList.add('active');
        else btn.classList.remove('active');
    }
}

function toggleVerbose() {
    state.verbose = !state.verbose;
    const btn = document.getElementById('toggle-verbose');
    if (btn) btn.classList.toggle('active');
    updateDiagnostics();
}

function updateDiagnostics() {
    const content = document.getElementById('report-content');
    if (!content || !state.data) return;
    content.textContent = state.verbose ? state.data.VerboseReport : state.data.Report;
}

async function performWhichSearch() {
    const input = document.getElementById('search-input');
    const query = input.value.trim();
    if (!query) {
        clearSearch();
        return;
    }

    document.getElementById('clear-search').style.display = 'block';
    state.searchTerm = query;
    try {
        const resp = await fetch(`/api/which?query=${encodeURIComponent(query)}`);
        if (!resp.ok) throw new Error("Search failed");
        const matches = await resp.json();
        state.mainFilteredIndices = matches.map(m => m.Index);
        state.whichMatches = matches;
        state.mainSelectedIndex = 0;
        renderAll();
    } catch (e) {
        console.error(e);
        const list = document.getElementById('path-list');
        if (list) list.innerHTML = `<p style="color:var(--warning); padding:20px;">Search Error: ${e.message}</p>`;
    }
}

function clearSearch() {
    state.mainFilteredIndices = state.data.PathEntries.map((_, i) => i);
    state.whichMatches = [];
    state.searchTerm = '';
    const input = document.getElementById('search-input');
    if (input) input.value = '';
    const clearBtn = document.getElementById('clear-search');
    if (clearBtn) clearBtn.style.display = 'none';
    renderAll();
}
