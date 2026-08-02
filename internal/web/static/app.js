'use strict';

const $ = (id) => document.getElementById(id);

// ---- Theme toggle (manual override of OS; applied before paint in <head>). ----
const themeSel = $('theme');
if (themeSel) {
  themeSel.value = localStorage.getItem('theme') || 'system';
  themeSel.addEventListener('change', () => {
    const v = themeSel.value;
    localStorage.setItem('theme', v);
    if (v === 'system') delete document.documentElement.dataset.theme;
    else document.documentElement.dataset.theme = v;
  });
}

// ---- Drag-drop assigns files to the real file input inside the form. ----
const drop = $('drop');
const fileInput = $('file');
if (drop && fileInput) {
  drop.addEventListener('click', () => fileInput.click());
  fileInput.addEventListener('change', updateDropLabel);
  ['dragenter', 'dragover'].forEach((ev) =>
    drop.addEventListener(ev, (e) => { e.preventDefault(); drop.classList.add('drag'); }));
  drop.addEventListener('dragleave', (e) => { e.preventDefault(); drop.classList.remove('drag'); });
  drop.addEventListener('drop', (e) => {
    e.preventDefault();
    drop.classList.remove('drag');
    fileInput.files = e.dataTransfer.files;
    updateDropLabel();
  });
}

function updateDropLabel() {
  const n = fileInput.files ? fileInput.files.length : 0;
  const label = $('drop-label');
  if (!label) return;
  label.textContent = n === 0 ? 'Drop books here, or choose files'
    : n === 1 ? fileInput.files[0].name
    : n + ' files selected';
}
