'use strict';

const $ = (id) => document.getElementById(id);

// ---- Theme ----
const themeSel = $('theme');
themeSel.value = localStorage.getItem('theme') || 'system';
themeSel.addEventListener('change', () => {
  const v = themeSel.value;
  localStorage.setItem('theme', v);
  if (v === 'system') document.documentElement.removeAttribute('data-theme');
  else document.documentElement.setAttribute('data-theme', v);
});

// ---- Identity ----
fetch('/me')
  .then((r) => r.json())
  .then((m) => { if (m.enabled && (m.name || m.user)) $('user').textContent = '👤 ' + (m.name || m.user); })
  .catch(() => {});

// ---- Defaults ----
fetch('/defaults')
  .then((r) => (r.ok ? r.text() : Promise.reject()))
  .then((yaml) => { $('raw_yaml').value = yaml; })
  .catch(() => { $('raw_yaml').placeholder = 'Could not load defaults'; });

// Keep the settings summary showing the chosen format.
const fmt = $('format');
const fmtLabel = $('fmt-label');
const syncFmt = () => { fmtLabel.textContent = '(' + fmt.value + ')'; };
fmt.addEventListener('change', syncFmt);
syncFmt();

// Upload a settings YAML into the raw editor.
$('yaml_upload').addEventListener('change', (e) => {
  const f = e.target.files[0];
  if (!f) return;
  f.text().then((t) => { $('raw_yaml').value = t; });
});

// ---- Drop area ----
const drop = $('drop');
const fileInput = $('file');
drop.addEventListener('click', () => fileInput.click());
fileInput.addEventListener('change', () => handleFiles(fileInput.files));
['dragenter', 'dragover'].forEach((ev) =>
  drop.addEventListener(ev, (e) => { e.preventDefault(); drop.classList.add('drag'); }));
drop.addEventListener('dragleave', (e) => { e.preventDefault(); drop.classList.remove('drag'); });
drop.addEventListener('drop', (e) => { e.preventDefault(); drop.classList.remove('drag'); handleFiles(e.dataTransfer.files); });

function optionalField(form, key, el, kind) {
  let v = '';
  if (kind === 'check') v = el.checked ? 'true' : '';
  else v = el.value;
  if (v !== '') form.append(key, v);
}

// booleanField always sends true/false so a pre-checked (default-on) box can be
// turned off. Omitting it would let the server-side default re-apply.
function booleanField(form, key, el) {
  form.append(key, el.checked ? 'true' : 'false');
}

function buildForm(file) {
  const form = new FormData();
  form.append('file', file);
  form.append('format', fmt.value);
  optionalField(form, 'toc_type', $('toc_type'));
  optionalField(form, 'footnotes_mode', $('footnotes_mode'));
  optionalField(form, 'jpeg_quality', $('jpeg_quality'));
  optionalField(form, 'images_optimize', $('images_optimize'), 'check');
  booleanField(form, 'insert_soft_hyphen', $('insert_soft_hyphen'));
  booleanField(form, 'cover_generate', $('cover_generate'));
  booleanField(form, 'dropcaps_enable', $('dropcaps_enable'));
  const raw = $('raw_yaml').value.trim();
  if (raw) form.append('raw_yaml', raw);
  return form;
}

// NOTE: browsers may block or prompt for multiple automatic downloads in a
// batch. This is acceptable for self-hosted use per the design.
function handleFiles(fileList) {
  for (const file of fileList) convertOne(file);
}

async function convertOne(file) {
  const safeName = escapeHtml(file.name);
  const li = document.createElement('li');
  li.textContent = `${file.name} — converting…`;
  $('results').prepend(li);
  try {
    const res = await fetch('/convert', { method: 'POST', body: buildForm(file) });
    if (!res.ok) {
      const msg = await res.text();
      li.innerHTML = `${safeName} — <span class="status-err">failed</span>: ${escapeHtml(msg.trim())}`;
      return;
    }
    const blob = await res.blob();
    const name = filenameFromDisposition(res.headers.get('Content-Disposition')) || (file.name + '.out');
    triggerDownload(blob, name);
    li.innerHTML = `${safeName} — <span class="status-ok">✓ downloaded ${escapeHtml(name)}</span>`;
  } catch (err) {
    li.innerHTML = `${safeName} — <span class="status-err">error</span>: ${escapeHtml(String(err))}`;
  }
}

function triggerDownload(blob, name) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = name;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 10000);
}

function filenameFromDisposition(cd) {
  if (!cd) return null;
  let m = /filename\*=UTF-8''([^;]+)/i.exec(cd);
  if (m) return decodeURIComponent(m[1]);
  m = /filename="?([^"]+)"?/i.exec(cd);
  return m ? m[1] : null;
}

function escapeHtml(s) {
  return s.replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}
