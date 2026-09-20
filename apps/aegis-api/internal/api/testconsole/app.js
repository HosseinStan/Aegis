(() => {
  const $ = (id) => document.getElementById(id);
  let token = sessionStorage.getItem('aegis_access_token') || '';
  let user = null;
  let devices = [];
  let credentials = [];

  async function api(path, options = {}, authenticate = true) {
    const headers = {...(options.headers || {})};
    if (options.body) headers['Content-Type'] = 'application/json';
    if (authenticate && token) headers.Authorization = `Bearer ${token}`;
    const response = await fetch(path, {...options, headers});
    const payload = response.status === 204 ? null : await response.json().catch(() => null);
    if (!response.ok) {
      const error = new Error(payload?.error?.message || `Request failed (${response.status})`);
      error.code = payload?.error?.code;
      if (response.status === 401 && authenticate) signOut();
      throw error;
    }
    return payload?.data ?? payload;
  }

  function showNotice(message, isError = false) {
    const box = $('notice'); box.textContent = message; box.hidden = false; box.classList.toggle('error', isError);
    window.clearTimeout(showNotice.timer); showNotice.timer = window.setTimeout(() => box.hidden = true, 8000);
  }
  function toggle(id) { const node = $(id); node.hidden = !node.hidden; }
  function signOut() {
    token = ''; user = null; sessionStorage.removeItem('aegis_access_token');
    $('workspace').hidden = true; $('loginPanel').hidden = false; $('sessionStatus').textContent = 'Signed out'; $('sessionStatus').className = 'pill offline';
  }

  async function login(event) {
    event.preventDefault(); $('loginError').textContent = '';
    try {
      const data = await api('/api/v1/auth/login', {method:'POST', body:JSON.stringify({username:$('username').value.trim(), password:$('password').value})}, false);
      token = data.tokens.access_token; user = data.user; sessionStorage.setItem('aegis_access_token', token); $('password').value = '';
      await enterWorkspace();
    } catch (error) { $('loginError').textContent = error.message; }
  }

  async function enterWorkspace() {
    if (!user) user = await api('/api/v1/auth/me');
    $('loginPanel').hidden = true; $('workspace').hidden = false;
    $('sessionStatus').textContent = 'Authenticated'; $('sessionStatus').className = 'pill online';
    const accessLabel = user.permissions.includes('system.manage') ? 'system administrator' : 'scoped operator';
    $('identity').textContent = `${user.username} · ${accessLabel} · ${user.permissions.length} permission grants`;
    await refreshAll();
  }

  async function refreshAll() {
    try {
      const [health, ready, devicePage, credentialPage, auditPage] = await Promise.all([
        api('/healthz', {}, false), api('/readyz', {}, false), api('/api/v1/devices'), api('/api/v1/credentials'), api('/api/v1/audit-logs?limit=8')
      ]);
      devices = Array.isArray(devicePage) ? devicePage : [];
      credentials = Array.isArray(credentialPage) ? credentialPage : [];
      $('health').textContent = health.status === 'ok' ? 'Healthy' : health.status;
      $('readiness').textContent = ready.status === 'ready' ? 'Ready' : ready.status;
      $('deviceCount').textContent = devices.length; $('credentialCount').textContent = credentials.length;
      renderCredentials(); renderDevices(); renderAudits(Array.isArray(auditPage) ? auditPage : []);
    } catch (error) { showNotice(error.message, true); }
  }

  function renderCredentials() {
    const list = $('credentials'); list.innerHTML = ''; list.classList.toggle('empty', credentials.length === 0);
    $('deviceCredential').innerHTML = credentials.length ? '<option value="">Select a credential</option>' : '<option value="">Create a credential first</option>';
    credentials.forEach(item => {
      const row = $('credentialTemplate').content.cloneNode(true); row.querySelector('.name').textContent = item.name; row.querySelector('.detail').textContent = `${item.auth_type} · ${item.username || 'username protected'}`; list.append(row);
      const option = document.createElement('option'); option.value = item.id; option.textContent = item.name; $('deviceCredential').append(option);
    });
    if (!credentials.length) list.textContent = 'No device credentials stored yet.';
  }

  function renderDevices() {
    const list = $('devices'); list.innerHTML = ''; list.classList.toggle('empty', devices.length === 0);
    devices.forEach(item => {
      const row = $('deviceTemplate').content.cloneNode(true); const article = row.querySelector('article'); article.dataset.id = item.id;
      row.querySelector('.name').textContent = item.display_name; row.querySelector('.detail').textContent = `${item.management_ip}:${item.management_port} · ${item.vendor} ${item.platform} · ${item.device_group || 'ungrouped'}`; list.append(row);
    });
    if (!devices.length) list.textContent = 'No devices registered yet. Add a credential, then register a lab switch.';
  }

  function renderAudits(items) {
    const list = $('audits'); list.innerHTML = ''; list.classList.toggle('empty', items.length === 0);
    items.forEach(item => { const row = document.createElement('article'); row.className = 'row'; const result = item.succeeded ? 'Succeeded' : `Failed: ${item.error_code || 'error'}`; row.innerHTML = `<div><strong></strong><small></small></div><small></small>`; row.querySelector('strong').textContent = item.action; row.querySelector('div small').textContent = `${item.username} · ${item.resource_type}`; row.querySelector(':scope > small').textContent = result; list.append(row); });
    if (!items.length) list.textContent = 'No audited activity yet.';
  }

  async function createCredential(event) {
    event.preventDefault();
    try {
      await api('/api/v1/credentials', {method:'POST', body:JSON.stringify({name:$('credentialName').value.trim(), auth_type:'password', username:$('credentialUsername').value.trim(), password:$('credentialPassword').value, enable_secret:$('enableSecret').value})});
      event.target.reset(); event.target.hidden = true; showNotice('Credential encrypted and stored.'); await refreshAll();
    } catch (error) { showNotice(error.message, true); }
  }

  async function createDevice(event) {
    event.preventDefault();
    try {
      await api('/api/v1/devices', {method:'POST', body:JSON.stringify({display_name:$('deviceName').value.trim(), management_ip:$('deviceIP').value.trim(), management_port:Number($('devicePort').value), vendor:'cisco', platform:$('devicePlatform').value, protocol:'ssh', credential_id:$('deviceCredential').value, device_group:$('deviceGroup').value.trim()})});
      event.target.reset(); $('devicePort').value = '22'; event.target.hidden = true; showNotice('Device registered. Use Test SSH when the host key is trusted.'); await refreshAll();
    } catch (error) { showNotice(error.message, true); }
  }

  async function deviceAction(event) {
    const button = event.target.closest('button[data-action]'); if (!button) return;
    const id = button.closest('[data-id]').dataset.id; const action = button.dataset.action; button.disabled = true;
    try {
      if (action === 'test') { await api(`/api/v1/devices/${id}/test-connection`, {method:'POST'}); showNotice('SSH connection succeeded.'); }
      if (action === 'config') { const result = await api(`/api/v1/devices/${id}/config/running`); showNotice(result.config); }
      if (action === 'backup') { const result = await api(`/api/v1/devices/${id}/backups`, {method:'POST', body:JSON.stringify({config_type:'running'})}); showNotice(result.created ? 'New running configuration backup created.' : 'Configuration is unchanged; existing backup retained.'); }
      await refreshAll();
    } catch (error) { showNotice(`${error.code ? error.code + ': ' : ''}${error.message}`, true); }
    finally { button.disabled = false; }
  }

  $('loginForm').addEventListener('submit', login); $('signOut').addEventListener('click', signOut); $('refreshAll').addEventListener('click', refreshAll);
  $('toggleCredential').addEventListener('click', () => toggle('credentialForm')); $('toggleDevice').addEventListener('click', () => toggle('deviceForm'));
  $('credentialForm').addEventListener('submit', createCredential); $('deviceForm').addEventListener('submit', createDevice); $('devices').addEventListener('click', deviceAction);
  if (token) enterWorkspace().catch(signOut);
})();
