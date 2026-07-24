(() => {
  const VERSION = "v1.2.0";
  const BASE = `https://github.com/zapabob/HermesDesktopwatchdog/releases/download/${VERSION}`;
  const ASSETS = {
    windows: `hermes-watchdog-windows-amd64-${VERSION}.tar.gz`,
    linux: `hermes-watchdog-linux-amd64-${VERSION}.tar.gz`,
    darwin: `hermes-watchdog-darwin-arm64-${VERSION}.tar.gz`,
  };

  const badge = document.getElementById("ver-badge");
  if (badge) badge.textContent = VERSION;

  const winBtn = document.getElementById("dl-windows");
  const linuxBtn = document.getElementById("dl-linux");
  const darwinBtn = document.getElementById("dl-darwin");
  const notesBtn = document.getElementById("release-notes");
  if (winBtn) winBtn.href = `${BASE}/${ASSETS.windows}`;
  if (linuxBtn) linuxBtn.href = `${BASE}/${ASSETS.linux}`;
  if (darwinBtn) darwinBtn.href = `${BASE}/${ASSETS.darwin}`;
  if (notesBtn) notesBtn.href = `https://github.com/zapabob/HermesDesktopwatchdog/releases/tag/${VERSION}`;

  const nameEl = document.getElementById("asset-name");
  const shaEl = document.getElementById("asset-sha");
  if (nameEl) nameEl.textContent = ASSETS.windows;

  fetch("checksums.json", { cache: "no-store" })
    .then((r) => (r.ok ? r.json() : null))
    .then((data) => {
      if (!shaEl || !data) {
        if (shaEl) shaEl.textContent = "see release SHA256SUMS";
        return;
      }
      const win = data.assets && data.assets.windows;
      const hash = (win && win.sha256) || data.sha256 || data.SHA256;
      shaEl.textContent = hash ? String(hash) : "see release SHA256SUMS";
      if (hash) shaEl.title = hash;
    })
    .catch(() => {
      if (shaEl) shaEl.textContent = "see release SHA256SUMS";
    });
})();
