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
    previewVisible: false,
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
        showLoading();
        const response = await fetch('/api/trace');
        state.data = await response.json();

        state.mainFilteredIndices = state.data.PathEntries.map((_, i) => i);

        document.getElementById('version-display').textContent = 'v' + state.data.Version;
        updateDiagnostics();

        renderAll();
        hideLoading();
    } catch (e) {
        console.error(e);
        hideLoading();
        const list = document.getElementById('path-list');
        if (list) list.textContent = 'Error loading trace: ' + e;
    }
}

function showLoading() {
    const overlay = document.getElementById('loading-overlay');
    const text = overlay.querySelector('.loading-text');
    text.textContent = 'Analyzing PATH...';
    overlay.classList.add('visible');
}

function hideLoading() {
    document.getElementById('loading-overlay').classList.remove('visible');
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
        // Preserve standard browser shortcuts (Ctrl+F, Cmd+R, etc.)
        if (e.ctrlKey || e.metaKey || e.altKey) return;

        // Global view toggles (shortcuts) - only if not typing in an input
        if (document.activeElement.tagName !== 'INPUT') {
            if (e.key === 'f' && !e.shiftKey) {
                e.preventDefault();
                switchView(state.currentView === 'flow' ? 'main' : 'flow');
                return;
            } else if (e.key === 'd' && !e.shiftKey) {
                e.preventDefault();
                switchView(state.currentView === 'diagnostics' ? 'main' : 'diagnostics');
                return;
            } else if ((e.key === 'h' && !e.shiftKey) || e.key === '?') {
                e.preventDefault();
                switchView(state.currentView === 'help' ? 'main' : 'help');
                return;
            }
        }

        if (state.currentView === 'diagnostics' && e.key !== 'd') return;

        if ((e.key === 'ArrowDown' || e.key === 'j') && !e.shiftKey) {
            e.preventDefault();
            moveSelection(1);
        } else if ((e.key === 'ArrowUp' || e.key === 'k') && !e.shiftKey) {
            e.preventDefault();
            moveSelection(-1);
        } else if (e.key === 'Escape') {
            clearSearch();
        } else if (state.currentView === 'flow') {
            if (e.key === 'ArrowLeft') {
                e.preventDefault();
                moveFlowNode(-1);
            } else if (e.key === 'ArrowRight' || (e.key === 'l' && !e.shiftKey)) {
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
    // Allow selecting shell start (-1) and shell ready (FlowNodes.length)
    const minIdx = -1;
    const maxIdx = state.data.FlowNodes.length;
    const newIdx = Math.max(minIdx, Math.min(state.flowNodeIndex + delta, maxIdx));
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
        updateToggles();
    } else if (viewId === 'help') {
        fetchHelp();
    }

    renderAll();
}

async function fetchHelp() {
    const content = document.getElementById('help-content');
    if (!content || content.dataset.loaded === 'true') return;

    try {
        const resp = await fetch('/api/help');
        if (!resp.ok) throw new Error("Server returned " + resp.status);
        const text = await resp.text();
        if (!text || text.trim() === '') throw new Error("Empty response from server");
        content.textContent = text;
        content.dataset.loaded = 'true';
    } catch (e) {
        console.error('Help loading error:', e);
        content.textContent = "Error loading help content: " + e.message + "\n\nTry refreshing the page or check the server logs.";
    }
}

function initializeFlowMode() {
    if (!state.data) return;
    
    // Default to "Shell Start" (flowNodeIndex = -1) which highlights nothing
    state.flowNodeIndex = -1;
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
            let isHighlighted = false;

            if (state.flowNodeIndex === -1) {
                // Shell Start: no highlighting
                isHighlighted = false;
            } else if (state.flowNodeIndex === state.data.FlowNodes.length) {
                // Shell Ready: highlight same as last node
                const lastNode = state.data.FlowNodes[state.data.FlowNodes.length - 1];
                if (state.cumulative) {
                    const nodeOfEntry = state.data.FlowNodes.find(n => n.ID === entry.FlowID);
                    if (nodeOfEntry && nodeOfEntry.Order <= lastNode.Order) {
                        isHighlighted = true;
                    }
                } else {
                    if (entry.FlowID === lastNode.ID) {
                        isHighlighted = true;
                    }
                }
            } else {
                // Normal flow node
                const activeNode = state.data.FlowNodes[state.flowNodeIndex];
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
            priority.style.fontSize = '1.1em';
            priority.style.color = 'var(--accent-bright)';
            priority.textContent = `(highest priority ${Icons.PriorityHigh})`;
            div.appendChild(priority);
        } else if (dataIdx === state.data.PathEntries.length - 1) {
            const priority = document.createElement('span');
            priority.style.marginLeft = '10px';
            priority.style.fontSize = '1.1em';
            priority.style.color = 'var(--text-muted)';
            priority.textContent = `(lowest priority ${Icons.PriorityLow})`;
            div.appendChild(priority);
        }

        if (entry.IsSessionOnly) {
            const status = document.createElement('span');
            status.className = 'status-pill';
            status.textContent = `session ${Icons.Session}`;
            div.appendChild(status);
        } else if (entry.IsDuplicate) {
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
                <div class="detail-label">Caused by</div>
                <div class="detail-value">
                    ${entry.SourceFile === 'System (Default)' && entry.LineNumber === 0 ? 'System (Default)' : `${entry.SourceFile}:${entry.LineNumber}`}
                    ${entry.Mode !== 'Unknown' ? `<span style="color:var(--text-muted); font-size:0.9em; margin-left:8px;">(Startup Phase: ${entry.Mode})</span>` : ''}
                </div>
            </div>
    `;

    // Always show source context section (educational for System defaults)
    if (entry.SourceFile === 'System (Default)' && entry.LineNumber === 0) {
        // Educational message for system default paths
        html += `
            <div class="detail-row" style="margin-top: 8px;">
                <div class="detail-label">Source Line Context</div>
                <div class="detail-value">
                    <div style="background: var(--bg-color); border: 1px solid var(--border); padding: 12px; border-radius: 6px; font-size: 0.9em; line-height: 1.6; color: var(--text-muted);">
                        <p style="margin: 0 0 8px 0;">This PATH entry comes from the <strong style="color: var(--text-color);">system default configuration</strong>, typically inherited from:</p>
                        <ul style="margin: 8px 0 8px 20px; padding: 0;">
                            <li style="margin: 4px 0;"><code style="color: var(--accent-bright);">/etc/paths</code> - System-wide PATH entries</li>
                            <li style="margin: 4px 0;"><code style="color: var(--accent-bright);">/etc/paths.d/*</code> - Additional system paths</li>
                            <li style="margin: 4px 0;">Built-in shell defaults</li>
                        </ul>
                        <p style="margin: 8px 0 0 0; font-size: 0.85em; font-style: italic;">This is normal and expected behavior on Unix-like systems. These paths are set before any user configuration files are processed.</p>
                    </div>
                </div>
            </div>
        `;
    } else {
        // Fetch and display the source line context for actual files
        try {
            const lineResp = await fetch(`/api/line-context?path=${encodeURIComponent(entry.SourceFile)}&line=${entry.LineNumber}`);
            if (lineResp.ok) {
                const lineContext = await lineResp.json();
                if (!lineContext.ErrorMsg) {
                    // Detect if/fi blocks and adjust which line to highlight
                    let highlightLineNum = lineContext.LineNumber;
                    const targetTrimmed = lineContext.Target.trim();
                    
                    if (targetTrimmed === 'fi') {
                        // Target is 'fi', look for corresponding 'if' and highlight content inside
                        if (lineContext.HasBefore2 && lineContext.Before2.trim().startsWith('if ')) {
                            // 'if' is 2 lines before, highlight the line in between (Before1)
                            highlightLineNum = lineContext.LineNumber - 1;
                        } else if (lineContext.HasBefore1 && lineContext.Before1.trim().startsWith('if ')) {
                            // 'if' is 1 line before (adjacent), highlight the 'if' line
                            highlightLineNum = lineContext.LineNumber - 1;
                        }
                    }
                    
                    html += `
                        <div class="detail-row" style="margin-top: 8px;">
                            <div class="detail-label">Source Line Context</div>
                            <div class="detail-value">
                                <div style="font-size: 0.85em; color: var(--text-muted); margin-bottom: 6px;">
                                    Filename: <span style="color: var(--text-color); font-family: monospace;">${entry.SourceFile}</span>
                                </div>
                                <div style="background: var(--bg-color); border: 1px solid var(--border); padding: 12px; border-radius: 6px; font-family: 'JetBrains Mono', monospace; font-size: 0.85em; line-height: 1.6; overflow-x: auto;">
                    `;
                    if (lineContext.HasBefore2) {
                        const isHighlight = (lineContext.LineNumber - 2) === highlightLineNum;
                        const style = isHighlight 
                            ? 'color: var(--accent-bright); font-weight: bold; background: rgba(122, 162, 247, 0.1); margin: 0 -12px; padding: 0 12px;'
                            : 'color: var(--text-muted);';
                        html += `<div style="${style}"> ${String(lineContext.LineNumber - 2).padStart(3, ' ')}  ${escapeHtml(lineContext.Before2)}</div>`;
                    }
                    if (lineContext.HasBefore1) {
                        const isHighlight = (lineContext.LineNumber - 1) === highlightLineNum;
                        const style = isHighlight 
                            ? 'color: var(--accent-bright); font-weight: bold; background: rgba(122, 162, 247, 0.1); margin: 0 -12px; padding: 0 12px;'
                            : 'color: var(--text-muted);';
                        html += `<div style="${style}"> ${String(lineContext.LineNumber - 1).padStart(3, ' ')}  ${escapeHtml(lineContext.Before1)}</div>`;
                    }
                    const isTargetHighlight = lineContext.LineNumber === highlightLineNum;
                    const targetStyle = isTargetHighlight
                        ? 'color: var(--accent-bright); font-weight: bold; background: rgba(122, 162, 247, 0.1); margin: 0 -12px; padding: 0 12px;'
                        : 'color: var(--text-muted);';
                    html += `<div style="${targetStyle}"> ${String(lineContext.LineNumber).padStart(3, ' ')}  ${escapeHtml(lineContext.Target)}</div>`;
                    if (lineContext.HasAfter1) {
                        const isHighlight = (lineContext.LineNumber + 1) === highlightLineNum;
                        const style = isHighlight 
                            ? 'color: var(--accent-bright); font-weight: bold; background: rgba(122, 162, 247, 0.1); margin: 0 -12px; padding: 0 12px;'
                            : 'color: var(--text-muted);';
                        html += `<div style="${style}"> ${String(lineContext.LineNumber + 1).padStart(3, ' ')}  ${escapeHtml(lineContext.After1)}</div>`;
                    }
                    if (lineContext.HasAfter2) {
                        const isHighlight = (lineContext.LineNumber + 2) === highlightLineNum;
                        const style = isHighlight 
                            ? 'color: var(--accent-bright); font-weight: bold; background: rgba(122, 162, 247, 0.1); margin: 0 -12px; padding: 0 12px;'
                            : 'color: var(--text-muted);';
                        html += `<div style="${style}"> ${String(lineContext.LineNumber + 2).padStart(3, ' ')}  ${escapeHtml(lineContext.After2)}</div>`;
                    }
                    html += `
                                </div>
                            </div>
                        </div>
                    `;
                }
            }
        } catch (e) {
            // Silently ignore if line context can't be fetched
        }
    }

    html += `</div>`;

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
        if (!resp.ok) {
            // Distinguish between non-existent and unreadable directories
            if (resp.status === 404 || resp.status === 500) {
                // Both 404 and 500 often indicate non-existent directory
                lsContainer.innerHTML = `<p style="color:var(--text-muted); padding:20px; font-style: italic;">Directory does not exist:<br><br><code>${entry.Value}</code></p>`;
            } else if (resp.status === 403) {
                lsContainer.innerHTML = `<p style="color:var(--warning); padding:20px;">⚠️ Permission denied: Cannot read directory <code>${entry.Value}</code></p>`;
            } else {
                lsContainer.innerHTML = `<p style="color:var(--warning); padding:20px;">⚠️ Cannot access directory (HTTP ${resp.status})</p>`;
            }
            return;
        }
        const files = await resp.json();
        state.currentLs = files;
        renderLsTable(files);
    } catch (e) {
        lsContainer.innerHTML = `<p style="color:var(--warning); padding:20px;">⚠️ Error: ${e.message}</p>`;
    }
}

function renderLsTable(files) {
    const container = document.getElementById('directory-listing-container');
    if (!container) return;

    // Handle null, undefined, or empty directory
    if (!files || files.length === 0) {
        container.innerHTML = '<p style="color:var(--text-muted); padding:20px; font-style: italic;">Directory is empty</p>';
        return;
    }

    let html = `
        <table class="listing-table">
            <thead>
                <tr>
                    <th class="col-permissions">Mode</th>
                    <th class="col-size">Size</th>
                    <th class="col-date">Modified</th>
                    <th class="col-name">Name</th>
                </tr>
            </thead>
            <tbody>
    `;

    files.forEach(f => {
        const sizeStr = formatSize(f.Size);
        const nameClass = f.IsDir ? 'listing-dir' : 'listing-name';
        html += `
            <tr>
                <td class="col-permissions">${f.Mode}</td>
                <td class="col-size">${sizeStr}</td>
                <td class="col-date">${f.ModTime}</td>
                <td class="col-name ${nameClass}">${f.IsDir ? '📁' : '📄'} ${f.Name}</td>
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

function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

function renderFlowNodes() {
    const container = document.getElementById('flow-nodes');
    if (!container) return;
    container.innerHTML = '';

    // Add Start Node (clickable, highlights nothing)
    const startNode = document.createElement('div');
    startNode.className = 'flow-node start-node' + (state.flowNodeIndex === -1 ? ' active' : '');
    startNode.textContent = '🚀 Shell Start';
    startNode.style.cursor = 'pointer';
    startNode.onclick = () => {
        state.flowNodeIndex = -1;
        renderAll();
    };
    container.appendChild(startNode);

    state.data.FlowNodes.forEach((node, idx) => {
        // Arrow separator
        const arrow = document.createElement('div');
        arrow.className = 'flow-arrow';
        arrow.textContent = '→';
        container.appendChild(arrow);

        const div = document.createElement('div');
        div.className = 'flow-node' + (idx === state.flowNodeIndex ? ' active' : '');
        
        // Clean up session node display - check BEFORE splitting
        let nodeText;
        if (node.FilePath === 'Session (Manual/Runtime)') {
            nodeText = 'Session ' + Icons.Session;
        } else {
            nodeText = node.FilePath.split('/').pop();
        }
        
        // Don't add first/last symbols in web view - position is obvious from GUI
        
        div.textContent = nodeText;
        div.title = node.FilePath;
        div.onclick = () => {
            state.flowNodeIndex = idx;
            renderAll();
        };
        container.appendChild(div);
    });

    // Add End Node (clickable, highlights same as last real node)
    const arrowEnd = document.createElement('div');
    arrowEnd.className = 'flow-arrow';
    arrowEnd.textContent = '→';
    container.appendChild(arrowEnd);

    const endNode = document.createElement('div');
    const isEndNodeActive = state.flowNodeIndex === state.data.FlowNodes.length;
    endNode.className = 'flow-node start-node' + (isEndNodeActive ? ' active' : '');
    endNode.textContent = '🏁 Shell Ready';
    endNode.style.cursor = 'pointer';
    endNode.onclick = () => {
        // Shell Ready highlights the same as the last real node
        state.flowNodeIndex = state.data.FlowNodes.length;
        renderAll();
    };
    container.appendChild(endNode);
}

function renderFlowPathList() {
    const allIndices = state.data.PathEntries.map((_, i) => i);
    
    // Filter indices based on flow node selection
    let filteredIndices = allIndices;
    if (state.flowNodeIndex === -1) {
        // Shell Start: show no highlighted paths
        filteredIndices = [];
    } else if (state.flowNodeIndex === state.data.FlowNodes.length) {
        // Shell Ready: same as last real node (cumulative)
        const lastNode = state.data.FlowNodes[state.data.FlowNodes.length - 1];
        if (state.cumulative) {
            // Highlight all paths from nodes up to and including the last
            filteredIndices = allIndices.filter(idx => {
                const entry = state.data.PathEntries[idx];
                const entryNode = state.data.FlowNodes.find(n => n.ID === entry.FlowID);
                return entryNode && entryNode.Order <= lastNode.Order;
            });
        } else {
            // Highlight only paths from the last node
            filteredIndices = allIndices.filter(idx => {
                const entry = state.data.PathEntries[idx];
                return entry.FlowID === lastNode.ID;
            });
        }
    }
    
    renderPathList('flow-path-list', allIndices, state.flowSelectedIndex, 'flow');
}

async function renderFilePreview() {
    const preview = document.getElementById('file-preview');
    const filenameLabel = document.getElementById('preview-filename');

    // Handle Shell Start node (index -1)
    if (state.flowNodeIndex === -1) {
        preview.classList.remove('file-content');
        filenameLabel.textContent = '🚀 Shell Start';
        preview.innerHTML = '<div style="color:var(--text-muted); padding:20px; line-height:1.6;"><p style="margin:0 0 12px 0;"><strong style="color:var(--text-color);">Shell initialization begins here.</strong></p><p style="margin:0;">At this point, no PATH entries have been loaded yet. The shell will begin reading configuration files based on its startup mode (login/interactive).</p></div>';
        return;
    }

    // Handle Shell Ready node (index === FlowNodes.length)
    if (state.flowNodeIndex === state.data.FlowNodes.length) {
        preview.classList.remove('file-content');
        filenameLabel.textContent = '🏁 Shell Ready';
        preview.innerHTML = '<div style="color:var(--text-muted); padding:20px; line-height:1.6;"><p style="margin:0 0 12px 0;"><strong style="color:var(--text-color);">Shell initialization complete.</strong></p><p style="margin:0;">All configuration files have been processed and the PATH is now fully constructed. The shell is ready for use.</p></div>';
        return;
    }

    const node = state.data.FlowNodes[state.flowNodeIndex];
    if (!node) return;

    filenameLabel.textContent = node.FilePath;

    // Handle special non-file nodes
    if (node.FilePath === 'Session (Manual/Runtime)') {
        preview.classList.remove('file-content');
        preview.innerHTML = '<div style="color:var(--text-muted); padding:20px; line-height:1.6;"><p style="margin:0 0 12px 0;"><strong style="color:var(--text-color);">Session paths ' + Icons.Session + '</strong> are added manually or by runtime tools, not from shell configuration files.</p><p style="margin:0;">These paths exist only in the current shell session and will not persist after the shell is closed unless added to a configuration file.</p></div>';
        return;
    }
    
    if (node.FilePath === 'System (Default)') {
        preview.classList.remove('file-content');
        preview.innerHTML = '<div style="color:var(--text-muted); padding:20px; line-height:1.6;"><p style="margin:0 0 12px 0;"><strong style="color:var(--text-color);">System default paths</strong> are inherited from system-wide configuration:</p><ul style="margin:8px 0 0 20px; padding:0;"><li style="margin:4px 0;"><code style="color:var(--accent-bright);">/etc/paths</code></li><li style="margin:4px 0;"><code style="color:var(--accent-bright);">/etc/paths.d/*</code></li><li style="margin:4px 0;">Built-in shell defaults</li></ul></div>';
        return;
    }

    if (node.NotExecuted) {
        preview.classList.remove('file-content');
        // Check if file exists by trying to fetch it
        try {
            const resp = await fetch(`/api/file?path=${encodeURIComponent(node.FilePath)}`);
            if (resp.ok) {
                preview.innerHTML = '<div style="color:var(--text-muted); padding:20px; line-height:1.6;"><p style="margin:0 0 12px 0;"><strong style="color:var(--text-color);">File not executed during this shell session.</strong></p><p style="margin:0;">File exists at: <code style="color:var(--accent-bright);">' + node.FilePath + '</code></p></div>';
            } else if (resp.status === 404) {
                preview.innerHTML = '<div style="color:var(--text-muted); padding:20px; line-height:1.6;"><p style="margin:0 0 12px 0;"><strong style="color:var(--text-color);">File not executed during this shell session.</strong></p><p style="margin:0;">File does not exist: <code style="color:var(--warning);">' + node.FilePath + '</code></p></div>';
            } else {
                preview.innerHTML = '<div style="color:var(--text-muted); padding:20px; line-height:1.6;"><p style="margin:0 0 12px 0;"><strong style="color:var(--text-color);">File not executed during this shell session.</strong></p><p style="margin:0;">Cannot access file (HTTP ' + resp.status + ')</p></div>';
            }
        } catch (e) {
            preview.innerHTML = '<div style="color:var(--text-muted); padding:20px; line-height:1.6;"><p style="margin:0 0 12px 0;"><strong style="color:var(--text-color);">File not executed during this shell session.</strong></p><p style="margin:0;">Error checking file: ' + e.message + '</p></div>';
        }
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
    preview.classList.add('file-content');
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
    const checkbox = document.getElementById('check-cumulative');
    if (checkbox) {
        state.cumulative = checkbox.checked;
    } else {
        state.cumulative = !state.cumulative;
    }
    renderAll();
}

function togglePreview() {
    const checkbox = document.getElementById('check-preview');
    if (checkbox) {
        state.previewVisible = checkbox.checked;
    } else {
        state.previewVisible = !state.previewVisible;
    }

    const preview = document.getElementById('preview-container');
    const splitter = document.getElementById('flow-splitter');
    const listPanel = document.getElementById('flow-list-panel');

    if (state.previewVisible) {
        preview.style.display = 'flex';
        splitter.style.display = 'block';
        listPanel.style.flex = '0 0 40%'; // Restore split to match CSS
    } else {
        preview.style.display = 'none';
        splitter.style.display = 'none';
        listPanel.style.flex = '1'; // Fill whole space
    }
}

function updateToggles() {
    const cumCheck = document.getElementById('check-cumulative');
    if (cumCheck) cumCheck.checked = state.cumulative;

    const preCheck = document.getElementById('check-preview');
    if (preCheck) preCheck.checked = state.previewVisible;

    // Apply preview visibility immediately
    const preview = document.getElementById('preview-container');
    const splitter = document.getElementById('flow-splitter');
    const listPanel = document.getElementById('flow-list-panel');

    if (preview && splitter && listPanel) {
        if (state.previewVisible) {
            preview.style.display = 'flex';
            splitter.style.display = 'block';
            listPanel.style.flex = '0 0 40%';
        } else {
            preview.style.display = 'none';
            splitter.style.display = 'none';
            listPanel.style.flex = '1';
        }
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

