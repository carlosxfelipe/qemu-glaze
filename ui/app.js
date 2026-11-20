let currentEditingId = "";
let systemInfo = null;
let savedVMs = [];
let vmStates = {};
let toastTimer = null;
let vmLogs = {};
let selectedLogVM = "";
let confirmResolver = null;

function showToast(msg) {
  const el = document.getElementById("adw-toast");
  el.textContent = msg;
  el.classList.add("visible");
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => el.classList.remove("visible"), 2800);
}

function showDeleteConfirm(vmName, hasDisk) {
  return new Promise((resolve) => {
    confirmResolver = resolve;
    document.getElementById("confirm-title").textContent = "Excluir Máquina";
    document.getElementById("confirm-message").textContent =
      `Deseja realmente remover a configuração da máquina "${vmName}"?`;
    const diskWrap = document.getElementById("confirm-disk-wrap");
    const diskCheck = document.getElementById("confirm-delete-disk");
    diskCheck.checked = false;
    diskWrap.style.display = hasDisk ? "flex" : "none";
    document.getElementById("confirm-modal").classList.remove("hidden");
  });
}

function resolveConfirmModal(confirmed) {
  document.getElementById("confirm-modal").classList.add("hidden");
  const deleteDisk = document.getElementById("confirm-delete-disk").checked;
  if (confirmResolver) {
    const fn = confirmResolver;
    confirmResolver = null;
    fn({ confirmed, deleteDisk });
  }
}

async function openExternalLink(url) {
  try {
    await window.api_open_external(url);
  } catch (_) {
    window.open(url, "_blank");
  }
}

function switchPage(page) {
  const titles = {
    vms: "Minhas Máquinas Virtuais",
    "new-vm": currentEditingId ? "Editar Máquina Virtual" : "Nova Máquina",
    logs: "Console & Logs",
    system: "Status do QEMU & Sistema",
    about: "Sobre o QEMU Glaze",
  };

  document.getElementById("page-title").textContent =
    titles[page] || "QEMU Glaze";

  ["vms", "new-vm", "logs", "system", "about"].forEach((p) => {
    const pageEl = document.getElementById("page-" + p);
    const navEl = document.getElementById("nav-" + p);
    if (pageEl) pageEl.classList.toggle("hidden", p !== page);
    if (navEl) navEl.classList.toggle("active", p === page);
  });

  if (page === "new-vm") {
    updateCmdPreview();
  }
}

function updateSliderProgress(input) {
  if (!input) return;
  const min = parseFloat(input.min) || 0;
  const max = parseFloat(input.max) || 100;
  const val = parseFloat(input.value) || 0;
  const pct =
    max > min
      ? Math.max(0, Math.min(100, ((val - min) / (max - min)) * 100))
      : 0;
  input.style.setProperty("--range-progress", pct + "%");
}

function onCoresSlider(val) {
  val = parseInt(val, 10) || 1;
  const slider = document.getElementById("vm-cores");
  updateSliderProgress(slider);
  document.getElementById("cores-badge").textContent =
    val + (val === 1 ? " Core" : " Cores");
  document.getElementById("cpu-sub-label").textContent =
    "Alocado: " + val + (val === 1 ? " Core" : " Cores");
  [1, 2, 4, 8].forEach((c) => {
    const chip = document.getElementById("chip-core-" + c);
    if (chip) chip.classList.toggle("active", val === c);
  });
  updateCmdPreview();
}

function setCoresPreset(c) {
  document.getElementById("vm-cores").value = c;
  onCoresSlider(c);
}

function onRAMSlider(val) {
  const mb = parseInt(val, 10);
  const slider = document.getElementById("vm-ram");
  updateSliderProgress(slider);
  const gb = (mb / 1024).toFixed(1);
  document.getElementById("ram-badge").textContent = mb + " MB";
  document.getElementById("ram-sub-label").textContent =
    "Alocado: " + mb + " MB (" + gb + " GB)";

  [1024, 2048, 4096, 8192].forEach((p) => {
    const chip = document.getElementById("chip-" + p);
    if (chip) chip.classList.toggle("active", mb === p);
  });

  updateCmdPreview();
}

function setRAMPreset(mb) {
  document.getElementById("vm-ram").value = mb;
  onRAMSlider(mb);
}

function onDiskSizeSlider(val) {
  val = parseInt(val, 10) || 1;
  const slider = document.getElementById("vm-disk-size");
  updateSliderProgress(slider);

  const freeGB =
    systemInfo && systemInfo.hostDiskFreeGB ? systemInfo.hostDiskFreeGB : 0;
  const pct = freeGB > 0 ? Math.round((val / freeGB) * 100) : 0;

  const badge = document.getElementById("disk-badge");
  if (badge) {
    badge.textContent = freeGB > 0 ? `${val} GB (${pct}%)` : `${val} GB`;
  }

  const label = document.getElementById("disk-size-label");
  if (label) {
    if (freeGB > 0 && val > freeGB) {
      label.innerHTML = `<span style="color: var(--destructive-color);">⚠️ ${val} GB excede o espaço livre no host (${freeGB} GB)</span>`;
    } else if (freeGB > 0) {
      const remaining = Math.max(freeGB - val, 0);
      label.textContent = `${val} GB (QCOW2 dinâmico • ${remaining} GB livres restantes no host)`;
    } else {
      label.textContent = val + " GB (alocação dinâmica QCOW2)";
    }
  }

  document.querySelectorAll("#disk-preset-chips .chip-btn").forEach((chip) => {
    const chipVal = parseInt(chip.getAttribute("data-val"), 10);
    chip.classList.toggle("active", chipVal === val);
  });

  updateCmdPreview();
}

function setDiskPreset(gb) {
  gb = parseInt(gb, 10);
  document.getElementById("vm-disk-size").value = gb;
  onDiskSizeSlider(gb);
}

function toggleDiskOptions() {
  const checked = document.getElementById("vm-create-disk").checked;
  document.getElementById("disk-size-row").style.display = checked
    ? "flex"
    : "none";
  updateCmdPreview();
}

function onArchChange() {
  const arch = document.getElementById("vm-arch").value;
  const hint = document.getElementById("arch-accel-hint");
  if (arch === "aarch64") {
    hint.textContent =
      systemInfo && systemInfo.os === "linux"
        ? "aarch64 utiliza KVM (ARM64) ou emulação rápida TCG"
        : "aarch64 utiliza aceleração nativa Apple Silicon HVF";
  } else {
    hint.textContent =
      systemInfo && systemInfo.os === "linux"
        ? "x86_64 utiliza aceleração KVM nativa no Linux"
        : "x86_64 utiliza emulação rápida TCG";
  }
  updateCmdPreview();
}

async function chooseISO() {
  try {
    const res = await window.api_browse_iso();
    if (res && res.path) {
      document.getElementById("vm-iso").value = res.path;
      if (!document.getElementById("vm-name").value) {
        const fileName = res.path
          .split("/")
          .pop()
          .replace(/\.[^/.]+$/, "");
        document.getElementById("vm-name").value = fileName.replace(
          /[-_]/g,
          " ",
        );
      }
      showToast("ISO selecionada: " + res.path.split("/").pop());
      updateCmdPreview();
    }
  } catch (err) {
    showToast("Erro ao abrir seletor: " + err);
  }
}

function clearISO() {
  document.getElementById("vm-iso").value = "";
  updateCmdPreview();
}

async function chooseExistingDisk() {
  try {
    const res = await window.api_browse_disk();
    if (res && res.path) {
      document.getElementById("vm-disk-path").value = res.path;
      document.getElementById("vm-create-disk").checked = false;
      toggleDiskOptions();
      showToast("Disco selecionado: " + res.path.split("/").pop());
      updateCmdPreview();
    }
  } catch (err) {
    showToast("Erro ao selecionar disco: " + err);
  }
}

async function updateCmdPreview() {
  const cfg = getFormData();
  try {
    const res = await window.api_build_command({ vm: cfg });
    if (res && res.command) {
      document.getElementById("cmd-preview-box").textContent = res.command;
      return;
    }
  } catch (_) {}

  const arch = cfg.arch;
  const bin = arch === "aarch64" ? "qemu-system-aarch64" : "qemu-system-x86_64";
  let str = bin + " -m " + cfg.memoryMB + "M -smp " + cfg.cores;
  if (arch === "aarch64") {
    str += " -M virt,highmem=on -accel hvf -cpu host -display cocoa";
  } else {
    str +=
      " -M q35 -accel tcg -cpu Nehalem -display cocoa -vga virtio -usb -device usb-tablet";
  }
  if (cfg.isoPath) {
    str += ' -cdrom "' + cfg.isoPath + '" -boot d';
  }
  document.getElementById("cmd-preview-box").textContent = str;
}

async function copyText(text, label) {
  try {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      await navigator.clipboard.writeText(text);
    } else {
      await window.api_copy_text(text);
    }
    showToast(label);
  } catch (_) {
    try {
      await window.api_copy_text(text);
      showToast(label);
    } catch (err) {
      showToast("Erro ao copiar: " + err);
    }
  }
}

function copyCommand() {
  const text = document.getElementById("cmd-preview-box").textContent.trim();
  copyText(text, "Comando copiado para a área de transferência!");
}

function getFormData() {
  const name =
    document.getElementById("vm-name").value.trim() || "Máquina Virtual";
  const arch = document.getElementById("vm-arch").value;
  const isoPath = document.getElementById("vm-iso").value.trim();
  const cores = parseInt(document.getElementById("vm-cores").value, 10) || 2;
  const memoryMB =
    parseInt(document.getElementById("vm-ram").value, 10) || 2048;
  const createDisk = document.getElementById("vm-create-disk").checked;
  const diskSizeGB =
    parseInt(document.getElementById("vm-disk-size").value, 10) || 20;
  const diskPath = document.getElementById("vm-disk-path").value.trim();

  return {
    id: currentEditingId || "",
    name,
    arch,
    cores,
    memoryMB,
    isoPath,
    createDisk,
    diskSizeGB,
    diskPath,
    enableAccel: true,
  };
}

function resetForm() {
  currentEditingId = "";
  document.getElementById("vm-name").value = "";
  document.getElementById("vm-iso").value = "";
  document.getElementById("vm-disk-path").value = "";
  document.getElementById("vm-arch").value =
    systemInfo && systemInfo.arch === "arm64" ? "aarch64" : "x86_64";
  document.getElementById("vm-create-disk").checked = true;
  toggleDiskOptions();
  setRAMPreset(2048);
  document.getElementById("vm-cores").value = 2;
  onCoresSlider(2);
  const defaultDisk =
    systemInfo && systemInfo.hostDiskFreeGB
      ? Math.min(20, systemInfo.hostDiskFreeGB)
      : 20;
  document.getElementById("vm-disk-size").value = defaultDisk;
  onDiskSizeSlider(defaultDisk);
  onArchChange();
  updateCmdPreview();
}

async function saveVMFromForm() {
  const cfg = getFormData();
  try {
    await window.api_save_machine({ vm: cfg });
    showToast("Máquina salva com sucesso!");
    resetForm();
    await loadAllVMs();
    switchPage("vms");
  } catch (err) {
    showToast("Erro ao salvar VM: " + err);
  }
}

async function startVMFromForm() {
  const cfg = getFormData();
  try {
    showToast("Iniciando QEMU...");
    const state = await window.api_start_machine({ adHoc: cfg });
    showToast("Máquina iniciada com sucesso!");
    selectedLogVM = (state && state.id) || cfg.id;
    await loadAllVMs();
    switchPage("vms");
  } catch (err) {
    showToast("Erro ao iniciar QEMU: " + err);
  }
}

async function startSavedVM(id) {
  try {
    showToast("Iniciando máquina...");
    await window.api_start_machine({ id });
    showToast("Máquina iniciada!");
    selectedLogVM = id;
    await loadAllVMs();
  } catch (err) {
    showToast("Falha ao iniciar: " + err);
  }
}

async function stopVM(id) {
  try {
    await window.api_stop_machine({ id, force: false });
    showToast("Máquina finalizada");
    await loadAllVMs();
  } catch (err) {
    showToast("Erro ao parar VM: " + err);
  }
}

async function deleteVM(id, name) {
  const vm = savedVMs.find((v) => v.id === id);
  const hasDisk = !!(vm && vm.diskPath);
  const { confirmed, deleteDisk } = await showDeleteConfirm(name, hasDisk);
  if (!confirmed) return;

  try {
    await window.api_delete_machine({ id, deleteDisk });
    showToast("Máquina excluída");
    await loadAllVMs();
  } catch (err) {
    showToast("Erro ao excluir: " + err);
  }
}

function editVM(id) {
  const vm = savedVMs.find((v) => v.id === id);
  if (!vm) return;

  currentEditingId = vm.id;
  document.getElementById("vm-name").value = vm.name || "";
  document.getElementById("vm-arch").value = vm.arch || "aarch64";
  document.getElementById("vm-iso").value = vm.isoPath || "";
  document.getElementById("vm-disk-path").value = vm.diskPath || "";
  document.getElementById("vm-cores").value = vm.cores || 2;
  onCoresSlider(vm.cores || 2);
  document.getElementById("vm-ram").value = vm.memoryMB || 2048;
  onRAMSlider(vm.memoryMB || 2048);

  document.getElementById("vm-create-disk").checked = !!vm.createDisk;
  document.getElementById("vm-disk-size").value = vm.diskSizeGB || 20;
  onDiskSizeSlider(vm.diskSizeGB || 20);
  toggleDiskOptions();
  onArchChange();

  switchPage("new-vm");
}

async function loadAllVMs() {
  try {
    const res = await window.api_list_machines();
    savedVMs = (res && res.items) || [];
    vmStates = (res && res.states) || {};
    renderVMsList();
    updateLogSelect();
  } catch (err) {
    console.error("Erro ao carregar VMs:", err);
  }
}

function renderVMsList() {
  const countEl = document.getElementById("vms-count");
  if (countEl) countEl.textContent = savedVMs.length;

  const container = document.getElementById("vms-container");
  if (!savedVMs.length) {
    container.innerHTML = `
      <div class="adw-empty-row">
        <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" style="opacity: 0.4;">
          <rect x="2" y="3" width="20" height="14" rx="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/>
        </svg>
        <div>
          <div style="font-weight: 600; font-size: 15px; margin-bottom: 4px; color: var(--window-fg-color);">Nenhuma Máquina Virtual configurada</div>
          <div>Crie sua primeira máquina virtual com ISO, RAM e CPU personalizadas.</div>
        </div>
        <button class="gtk-btn suggested-action" style="margin-top: 8px;" onclick="switchPage('new-vm')">
          Criar Nova Máquina
        </button>
      </div>
    `;
    return;
  }

  container.innerHTML = savedVMs
    .map((vm) => {
      const state = vmStates[vm.id] || {
        status: "stopped",
        pid: 0,
        uptimeSec: 0,
      };
      const isRunning = state.status === "running";
      const isError = state.status === "error";

      let statusBadge = `<span class="gtk-pill stopped">Parada</span>`;
      if (isRunning) {
        statusBadge = `<span class="gtk-pill running"><span class="pulse-dot"></span> Executando (PID ${state.pid})</span>`;
      } else if (isError) {
        statusBadge = `<span class="gtk-pill error">Erro</span>`;
      }

      const ramGB = (vm.memoryMB / 1024).toFixed(1);
      const isoDisplay = vm.isoPath
        ? vm.isoPath.split("/").pop()
        : "Nenhuma ISO (Live/PXE)";
      const diskBadge = vm.diskPath
        ? `<span class="gtk-pill" title="${escapeHTML(vm.diskPath)}">💽 ${escapeHTML(vm.diskPath.split("/").pop())}${vm.createDisk && vm.diskSizeGB ? ` (${vm.diskSizeGB} GB)` : ""}</span>`
        : vm.createDisk
          ? `<span class="gtk-pill">💽 QCOW2 (${vm.diskSizeGB || 20} GB)</span>`
          : "";

      return `
        <div class="vm-card">
          <div class="vm-card-header">
            <div>
              <span class="vm-card-title">${escapeHTML(vm.name)}</span>
              <span style="font-size: 12px; color: var(--dim-label-color); margin-left: 8px;">${vm.createdAt || ""}</span>
            </div>
            <div>${statusBadge}</div>
          </div>

          <div class="vm-card-details">
            <span class="gtk-pill info">${vm.arch}</span>
            <span class="gtk-pill">${vm.cores} ${vm.cores === 1 ? "Core" : "Cores"}</span>
            <span class="gtk-pill">${vm.memoryMB} MB (${ramGB} GB RAM)</span>
            <span class="gtk-pill" title="${escapeHTML(vm.isoPath || "")}">💿 ${escapeHTML(isoDisplay)}</span>
            ${diskBadge}
          </div>

          <div class="vm-card-actions">
            <div style="display: flex; gap: 8px;">
              ${
                isRunning
                  ? `<button class="gtk-btn destructive-action" onclick="stopVM('${vm.id}')">
                      <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor"><rect x="6" y="6" width="12" height="12"/></svg>
                      Parar
                    </button>`
                  : `<button class="gtk-btn suggested-action" onclick="startSavedVM('${vm.id}')">
                      <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor"><polygon points="5 3 19 12 5 21 5 3"/></svg>
                      Iniciar
                    </button>`
              }
              <button class="gtk-btn" title="Ver logs" onclick="viewVMLogs('${vm.id}')">Logs</button>
            </div>

            <div style="display: flex; gap: 6px;">
              <button class="gtk-btn flat" title="Editar máquina" onclick="editVM('${vm.id}')">
                <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M12 20h9"/><path d="M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z"/>
                </svg>
              </button>
              <button class="gtk-btn flat destructive-action" title="Excluir máquina" onclick="deleteVM('${vm.id}', '${escapeHTML(vm.name)}')">
                <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="var(--destructive-color)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <polyline points="3 6 5 6 21 6"/><path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/><path d="M10 11v6"/><path d="M14 11v6"/><path d="M9 6V4a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2"/>
                </svg>
              </button>
            </div>
          </div>
        </div>
      `;
    })
    .join("");
}

async function viewVMLogs(id) {
  selectedLogVM = id;
  document.getElementById("log-vm-select").value = id;
  try {
    const res = await window.api_get_logs(id);
    if (
      res &&
      res.lines &&
      res.lines.length &&
      (!vmLogs[id] || !vmLogs[id].length)
    ) {
      vmLogs[id] = res.lines;
    }
  } catch (_) {}
  renderTerminal();
  switchPage("logs");
}

function updateLogSelect() {
  const sel = document.getElementById("log-vm-select");
  const current = sel.value;
  sel.innerHTML =
    '<option value="">Selecione uma máquina...</option>' +
    savedVMs
      .map((v) => `<option value="${v.id}">${escapeHTML(v.name)}</option>`)
      .join("");
  if (current) sel.value = current;
}

function switchLogVM() {
  selectedLogVM = document.getElementById("log-vm-select").value;
  renderTerminal();
}

function renderTerminal() {
  const term = document.getElementById("terminal-content");
  if (!selectedLogVM) {
    term.textContent =
      "[QEMU Glaze] Nenhuma máquina selecionada para visualização de logs.";
    return;
  }
  const logs = vmLogs[selectedLogVM] || [];
  if (!logs.length) {
    term.textContent = `[QEMU Glaze] Aguardando saída do console para a máquina ${selectedLogVM}...`;
    return;
  }
  term.textContent = logs.join("\n");
  term.scrollTop = term.scrollHeight;
}

function clearCurrentLogs() {
  if (selectedLogVM) {
    vmLogs[selectedLogVM] = [];
    renderTerminal();
  }
}

function copyCurrentLogs() {
  const term = document.getElementById("terminal-content");
  copyText(term.textContent, "Logs copiados!");
}

async function openDisksFolder() {
  if (systemInfo && systemInfo.disksDir) {
    await openExternalLink(systemInfo.disksDir);
  }
}

function setStatusPill(id, installed, labelOk, labelMissing) {
  const el = document.getElementById(id);
  if (!el) return;
  el.textContent = installed ? labelOk : labelMissing;
  el.className = "gtk-pill " + (installed ? "running" : "stopped");
}

async function initSystem() {
  try {
    const info = await window.api_system_info();
    systemInfo = info;

    document.getElementById("host-arch-label").textContent =
      `${info.os} (${info.arch})`;
    const ramGB = (info.hostMemoryMB / 1024).toFixed(0);
    document.getElementById("host-ram-label").textContent =
      `${ramGB} GB RAM (${info.hostCpus} CPUs)`;
    if (info.hostDiskFreeGB > 0) {
      document.getElementById("host-disk-label").textContent =
        `${info.hostDiskFreeGB} GB livres`;
    }

    const qemuDetected = !!(info.qemuArmPath || info.qemuX86Path);
    const qPill = document.getElementById("qemu-status-pill");
    qPill.textContent = qemuDetected ? "Detectado" : "Ausente";
    qPill.style.color = qemuDetected
      ? "var(--success-color)"
      : "var(--destructive-color)";

    const coresSlider = document.getElementById("vm-cores");
    coresSlider.max = Math.max(info.hostCpus, 4);
    document.getElementById("max-cores-label").textContent =
      `${coresSlider.max} Cores`;

    const ramSlider = document.getElementById("vm-ram");
    ramSlider.max = Math.min(info.hostMemoryMB, 32768);
    document.getElementById("max-ram-label").textContent =
      `${ramSlider.max} MB`;

    const diskSlider = document.getElementById("vm-disk-size");
    if (info.hostDiskFreeGB > 0) {
      const maxFree = info.hostDiskFreeGB;
      diskSlider.min = 1;
      diskSlider.step = 1;
      diskSlider.max = maxFree;

      document.getElementById("min-disk-label").textContent = "1 GB";
      document.getElementById("max-disk-label").textContent =
        `${maxFree} GB (100% Livre)`;
      document.getElementById("disk-free-label").textContent =
        `Disponível no host: ${maxFree} GB livres`;

      const chipsContainer = document.getElementById("disk-preset-chips");
      if (chipsContainer) {
        let presets = [];
        if (maxFree <= 15) {
          presets = [
            { label: "25%", gb: Math.max(Math.round(maxFree * 0.25), 1) },
            { label: "50%", gb: Math.max(Math.round(maxFree * 0.5), 1) },
            { label: "75%", gb: Math.max(Math.round(maxFree * 0.75), 1) },
            { label: `100% (${maxFree} GB)`, gb: maxFree },
          ];
        } else {
          const p1 = 10;
          const p2 = Math.min(20, Math.floor(maxFree * 0.6));
          const half = Math.round(maxFree * 0.5);
          const p3 =
            p2 === half || p1 === half ? Math.round(maxFree * 0.75) : half;
          const p4 = maxFree;
          presets = [
            { label: `${p1} GB`, gb: p1 },
            { label: `${p2} GB`, gb: p2 },
            {
              label: p3 === half ? `50% (${p3} GB)` : `75% (${p3} GB)`,
              gb: p3,
            },
            { label: `Máx (${p4} GB)`, gb: p4 },
          ];
        }

        const unique = [];
        const seen = new Set();
        for (const p of presets) {
          if (!seen.has(p.gb) && p.gb >= 1 && p.gb <= maxFree) {
            seen.add(p.gb);
            unique.push(p);
          }
        }

        chipsContainer.innerHTML = unique
          .map(
            (p) => `
            <button type="button" class="chip-btn ${parseInt(diskSlider.value, 10) === p.gb ? "active" : ""}" data-val="${p.gb}" onclick="setDiskPreset(${p.gb})">
              ${p.label}
            </button>
          `,
          )
          .join("");
      }

      if (parseInt(diskSlider.value, 10) > diskSlider.max) {
        diskSlider.value = diskSlider.max;
      }
    }

    updateSliderProgress(coresSlider);
    updateSliderProgress(ramSlider);
    updateSliderProgress(diskSlider);
    onDiskSizeSlider(diskSlider.value);

    document.getElementById("sys-qemu-version").textContent =
      info.qemuVersion || "Não encontrado";
    document.getElementById("sys-version-pill").textContent = qemuDetected
      ? "QEMU"
      : "Indisponível";
    document.getElementById("sys-arm-path").textContent =
      info.qemuArmPath || "Não instalado";
    document.getElementById("sys-x86-path").textContent =
      info.qemuX86Path || "Não instalado";
    document.getElementById("sys-img-path").textContent =
      info.qemuImgPath || "Não instalado";
    document.getElementById("sys-edk2-path").textContent =
      info.edk2ArmPath || "Firmware padrão QEMU";

    setStatusPill("sys-arm-pill", !!info.qemuArmPath, "Instalado", "Ausente");
    setStatusPill("sys-x86-pill", !!info.qemuX86Path, "Instalado", "Ausente");
    setStatusPill("sys-img-pill", !!info.qemuImgPath, "Instalado", "Ausente");
    setStatusPill("sys-edk2-pill", !!info.edk2ArmPath, "Pronto", "Padrão");

    const diskDiagText =
      info.hostDiskFreeGB > 0
        ? `${info.disksDir || "~/.qemu-glaze/disks"} (${info.hostDiskFreeGB} GB livres de ${info.hostDiskTotalGB} GB)`
        : info.disksDir || "~/.qemu-glaze/disks";
    document.getElementById("sys-disks-dir").textContent = diskDiagText;

    document.getElementById("vm-arch").value =
      info.arch === "arm64" ? "aarch64" : "x86_64";
    onArchChange();
  } catch (err) {
    console.error("Falha ao inicializar sistema:", err);
  }

  await loadAllVMs();
}

function escapeHTML(str) {
  return String(str || "").replace(
    /[&<>"']/g,
    (s) =>
      ({
        "&": "&amp;",
        "<": "&lt;",
        ">": "&gt;",
        '"': "&quot;",
        "'": "&#39;",
      })[s],
  );
}

if (window.glaze && glaze.events) {
  glaze.events.on("vms:status", (state) => {
    if (state && state.id) {
      vmStates[state.id] = state;
      renderVMsList();
    }
  });

  glaze.events.on("vms:telemetry", (states) => {
    if (states) {
      vmStates = states;
      renderVMsList();
    }
  });

  glaze.events.on("vms:log", (data) => {
    if (data && data.id) {
      if (!vmLogs[data.id]) vmLogs[data.id] = [];
      vmLogs[data.id].push(`[${data.time}] ${data.line}`);
      if (vmLogs[data.id].length > 500) {
        vmLogs[data.id].shift();
      }
      if (selectedLogVM === data.id) {
        renderTerminal();
      }
    }
  });

  glaze.events.on("app:navigate", (page) => {
    if (page) switchPage(page);
  });

  glaze.events.on("app:refresh", () => {
    loadAllVMs();
  });
}

window.addEventListener("DOMContentLoaded", initSystem);
