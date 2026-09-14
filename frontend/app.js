/**
 * WhatsApp Full Profile Picture - Frontend Application
 */

// Application State
const state = {
  currentImageBase64: null,
  connectionMethod: 'qr', // 'qr' or 'phone'
  socket: null,
  backendUrl: localStorage.getItem('wa_pfp_backend_url') || '',
  isProcessing: false,
};

// DOM Elements
const dropZone = document.getElementById('drop-zone');
const fileInput = document.getElementById('file-input');
const dropEmpty = document.getElementById('drop-empty');
const dropPreview = document.getElementById('drop-preview');
const previewImage = document.getElementById('preview-image');
const fileNameDisplay = document.getElementById('file-name-display');
const fileDimDisplay = document.getElementById('file-dim-display');
const btnChangeImage = document.getElementById('btn-change-image');
const btnStart = document.getElementById('btn-start-process');

const tabQr = document.getElementById('tab-qr');
const tabPhone = document.getElementById('tab-phone');
const phoneInputGroup = document.getElementById('phone-input-group');
const inputPhoneNumber = document.getElementById('input-phone-number');

// Views & Steps
const viewUpload = document.getElementById('view-upload');
const viewScan = document.getElementById('view-scan');
const viewSuccess = document.getElementById('view-success');

const stepNav1 = document.getElementById('step-nav-1');
const stepNav2 = document.getElementById('step-nav-2');
const stepNav3 = document.getElementById('step-nav-3');
const stepLine1 = document.getElementById('step-line-1');
const stepLine2 = document.getElementById('step-line-2');

// Scan View Elements
const qrBox = document.getElementById('qr-box');
const qrImage = document.getElementById('qr-image');
const qrSpinner = document.getElementById('qr-loading-spinner');
const pairingBox = document.getElementById('pairing-box');
const pairingCodeText = document.getElementById('pairing-code-text');
const liveStatusMessage = document.getElementById('live-status-message');
const btnCancelSession = document.getElementById('btn-cancel-session');
const btnRestart = document.getElementById('btn-restart');

// Settings Elements
const serverStatusBtn = document.getElementById('server-status-btn');
const serverStatusText = document.getElementById('server-status-text');
const btnOpenSettings = document.getElementById('btn-open-settings');
const settingsModal = document.getElementById('settings-modal');
const btnCloseModal = document.getElementById('btn-close-modal');
const inputBackendUrl = document.getElementById('input-backend-url');
const btnSaveSettings = document.getElementById('btn-save-settings');

/* ==========================================================================
   Backend URL & WebSocket Helpers
   ========================================================================== */
function getBackendHttpUrl() {
  if (state.backendUrl && state.backendUrl.trim() !== '') {
    return state.backendUrl.trim().replace(/\/+$/, '');
  }
  return window.location.origin;
}

function getBackendWsUrl() {
  const httpUrl = getBackendHttpUrl();
  if (httpUrl.startsWith('https://')) {
    return 'wss://' + httpUrl.replace('https://', '') + '/ws';
  } else if (httpUrl.startsWith('http://')) {
    return 'ws://' + httpUrl.replace('http://', '') + '/ws';
  }
  // Fallback
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${protocol}//${window.location.host}/ws`;
}

async function checkBackendHealth() {
  const base = getBackendHttpUrl();
  serverStatusBtn.className = 'status-pill status-checking';
  serverStatusText.textContent = 'Memeriksa Backend...';

  try {
    const res = await fetch(`${base}/health`, { method: 'GET', mode: 'cors' });
    if (res.ok) {
      serverStatusBtn.className = 'status-pill status-online';
      serverStatusText.textContent = 'Backend Online';
    } else {
      throw new Error(`Status ${res.status}`);
    }
  } catch (err) {
    serverStatusBtn.className = 'status-pill status-offline';
    serverStatusText.textContent = 'Backend Offline';
  }
}

/* ==========================================================================
   File Upload & Drag-and-Drop
   ========================================================================== */
function initDropZone() {
  // Click drop zone opens file picker
  dropZone.addEventListener('click', (e) => {
    if (e.target !== btnChangeImage && !btnChangeImage.contains(e.target)) {
      fileInput.click();
    }
  });

  btnChangeImage.addEventListener('click', (e) => {
    e.stopPropagation();
    fileInput.click();
  });

  // Drag over
  ['dragenter', 'dragover'].forEach(name => {
    dropZone.addEventListener(name, (e) => {
      e.preventDefault();
      e.stopPropagation();
      dropZone.classList.add('drag-over');
    });
  });

  // Drag leave
  ['dragleave', 'drop'].forEach(name => {
    dropZone.addEventListener(name, (e) => {
      e.preventDefault();
      e.stopPropagation();
      dropZone.classList.remove('drag-over');
    });
  });

  // Handle dropped file
  dropZone.addEventListener('drop', (e) => {
    const files = e.dataTransfer.files;
    if (files.length > 0) {
      handleSelectedFile(files[0]);
    }
  });

  // Handle file input change
  fileInput.addEventListener('change', () => {
    if (fileInput.files.length > 0) {
      handleSelectedFile(fileInput.files[0]);
    }
  });
}

function handleSelectedFile(file) {
  if (!file.type.startsWith('image/')) {
    alert('File harus berupa gambar (JPG, PNG, atau WEBP).');
    return;
  }

  fileNameDisplay.textContent = file.name;

  const reader = new FileReader();
  reader.onload = (e) => {
    const rawData = e.target.result;
    const tempImg = new Image();
    tempImg.onload = () => {
      let w = tempImg.naturalWidth;
      let h = tempImg.naturalHeight;
      fileDimDisplay.textContent = `${w} x ${h} px`;

      // Scale to max 1080x1920 for fast network transfer
      const maxW = 1080;
      const maxH = 1920;
      if (w > maxW || h > maxH) {
        const ratio = Math.min(maxW / w, maxH / h);
        w = Math.round(w * ratio);
        h = Math.round(h * ratio);
      }

      const canvas = document.createElement('canvas');
      canvas.width = w;
      canvas.height = h;
      const ctx = canvas.getContext('2d');
      ctx.drawImage(tempImg, 0, 0, w, h);

      // Store optimized JPEG (quality 0.95)
      state.currentImageBase64 = canvas.toDataURL('image/jpeg', 0.95);
      previewImage.src = state.currentImageBase64;
      btnStart.disabled = false;

      dropEmpty.classList.add('hidden');
      dropPreview.classList.remove('hidden');
    };
    tempImg.src = rawData;
  };
  reader.readAsDataURL(file);
}

/* ==========================================================================
   Tab / Method Selection
   ========================================================================== */
function initTabs() {
  tabQr.addEventListener('click', () => {
    state.connectionMethod = 'qr';
    tabQr.classList.add('active');
    tabPhone.classList.remove('active');
    phoneInputGroup.classList.add('hidden');
  });

  tabPhone.addEventListener('click', () => {
    state.connectionMethod = 'phone';
    tabPhone.classList.add('active');
    tabQr.classList.remove('active');
    phoneInputGroup.classList.remove('hidden');
    inputPhoneNumber.focus();
  });
}

/* ==========================================================================
   View Navigation
   ========================================================================== */
function setStep(stepNumber) {
  // Hide all views
  viewUpload.classList.add('hidden');
  viewScan.classList.add('hidden');
  viewSuccess.classList.add('hidden');

  // Reset step pills
  stepNav1.classList.remove('active');
  stepNav2.classList.remove('active');
  stepNav3.classList.remove('active');
  stepLine1.classList.remove('active');
  stepLine2.classList.remove('active');

  if (stepNumber === 1) {
    viewUpload.classList.remove('hidden');
    stepNav1.classList.add('active');
  } else if (stepNumber === 2) {
    viewScan.classList.remove('hidden');
    stepNav1.classList.add('active');
    stepNav2.classList.add('active');
    stepLine1.classList.add('active');
  } else if (stepNumber === 3) {
    viewSuccess.classList.remove('hidden');
    stepNav1.classList.add('active');
    stepNav2.classList.add('active');
    stepNav3.classList.add('active');
    stepLine1.classList.add('active');
    stepLine2.classList.add('active');
  }
}

/* ==========================================================================
   WebSocket & Process Execution
   ========================================================================== */
function startProcess() {
  if (!state.currentImageBase64) {
    alert('Silakan pilih foto terlebih dahulu.');
    return;
  }

  let pairNumber = '';
  if (state.connectionMethod === 'phone') {
    pairNumber = inputPhoneNumber.value.trim().replace(/\D+/g, '');
    if (!pairNumber || pairNumber.length < 9) {
      alert('Masukkan nomor WhatsApp yang valid (termasuk kode negara, misal: 628123456789).');
      inputPhoneNumber.focus();
      return;
    }
  }

  // Switch to Scan View
  setStep(2);
  state.isProcessing = true;

  // Reset QR & Pairing displays
  qrImage.classList.add('hidden');
  qrSpinner.classList.remove('hidden');
  liveStatusMessage.textContent = 'Menghubungkan ke server...';

  if (state.connectionMethod === 'phone') {
    qrBox.classList.add('hidden');
    pairingBox.classList.remove('hidden');
    pairingCodeText.textContent = '------';
  } else {
    qrBox.classList.remove('hidden');
    pairingBox.classList.add('hidden');
  }

  const wsUrl = getBackendWsUrl();
  console.log('Connecting to WebSocket:', wsUrl);

  try {
    state.socket = new WebSocket(wsUrl);
  } catch (err) {
    handleError('Gagal membuka koneksi WebSocket: ' + err.message);
    return;
  }

  state.socket.onopen = () => {
    liveStatusMessage.textContent = 'Mengirim gambar ke server...';
    const payload = {
      action: 'start',
      image: state.currentImageBase64,
      pair_number: pairNumber,
    };
    state.socket.send(JSON.stringify(payload));
  };

  state.socket.onmessage = (event) => {
    try {
      const msg = JSON.parse(event.data);
      handleServerMessage(msg);
    } catch (err) {
      console.error('Invalid message from server:', event.data);
    }
  };

  state.socket.onerror = (err) => {
    console.error('WebSocket error:', err);
    handleError('Terjadi kesalahan koneksi ke server backend. Pastikan backend aktif.');
  };

  state.socket.onclose = (e) => {
    console.log('WebSocket closed:', e.code, e.reason);
    if (state.isProcessing) {
      const detail = e.reason ? `: ${e.reason}` : (e.code ? ` (Kode: ${e.code})` : '');
      handleError(`Koneksi terputus dari server${detail}.`);
    }
  };
}

function handleServerMessage(msg) {
  console.log('Server message:', msg);

  if (msg.message) {
    liveStatusMessage.textContent = msg.message;
  }

  switch (msg.type) {
    case 'status':
      // Status update is already rendered above
      break;

    case 'qr':
      // Display QR code image
      if (msg.qr_image) {
        qrImage.src = msg.qr_image;
        qrImage.classList.remove('hidden');
        qrSpinner.classList.add('hidden');
      }
      break;

    case 'pairing_code':
      // Display pairing code
      if (msg.code) {
        pairingCodeText.textContent = formatPairingCode(msg.code);
      }
      break;

    case 'success':
      state.isProcessing = false;
      setStep(3);
      break;

    case 'error':
      handleError(msg.message || 'Terjadi kesalahan pada server WhatsApp.');
      break;
  }
}

function formatPairingCode(code) {
  // Format code to readable 4-4 digits if length 8
  const clean = code.replace(/\s+/g, '');
  if (clean.length === 8) {
    return clean.slice(0, 4) + ' - ' + clean.slice(4);
  }
  return clean;
}

function handleError(errorMessage) {
  state.isProcessing = false;
  if (state.socket) {
    state.socket.close();
    state.socket = null;
  }
  alert(errorMessage);
  setStep(1);
}

function cancelSession() {
  if (confirm('Yakin ingin membatalkan proses pemasangan foto?')) {
    state.isProcessing = false;
    if (state.socket) {
      state.socket.close();
      state.socket = null;
    }
    setStep(1);
  }
}

function restartApp() {
  state.isProcessing = false;
  if (state.socket) {
    state.socket.close();
    state.socket = null;
  }
  fileInput.value = '';
  state.currentImageBase64 = null;
  dropEmpty.classList.remove('hidden');
  dropPreview.classList.add('hidden');
  btnStart.disabled = true;
  setStep(1);
}

/* ==========================================================================
   Settings Modal
   ========================================================================== */
function initSettings() {
  inputBackendUrl.value = state.backendUrl;

  const openModal = () => {
    inputBackendUrl.value = state.backendUrl;
    settingsModal.classList.remove('hidden');
  };

  const closeModal = () => {
    settingsModal.classList.add('hidden');
  };

  btnOpenSettings.addEventListener('click', openModal);
  serverStatusBtn.addEventListener('click', openModal);
  btnCloseModal.addEventListener('click', closeModal);

  settingsModal.addEventListener('click', (e) => {
    if (e.target === settingsModal) {
      closeModal();
    }
  });

  btnSaveSettings.addEventListener('click', () => {
    const val = inputBackendUrl.value.trim();
    state.backendUrl = val;
    if (val) {
      localStorage.setItem('wa_pfp_backend_url', val);
    } else {
      localStorage.removeItem('wa_pfp_backend_url');
    }
    closeModal();
    checkBackendHealth();
  });
}

/* ==========================================================================
   Initialization
   ========================================================================== */
document.addEventListener('DOMContentLoaded', () => {
  initDropZone();
  initTabs();
  initSettings();

  btnStart.addEventListener('click', startProcess);
  btnCancelSession.addEventListener('click', cancelSession);
  btnRestart.addEventListener('click', restartApp);

  // Check health on start and periodically
  checkBackendHealth();
  setInterval(checkBackendHealth, 20000);
});
