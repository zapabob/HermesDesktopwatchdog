(() => {
  const VERSION = "v1.0.0";
  const ASSET = `hermes-watchdog-windows-amd64-${VERSION}.tar.gz`;
  const RELEASE_ASSET_URL = `https://github.com/zapabob/HermesDesktopwatchdog/releases/download/${VERSION}/${ASSET}`;
  const CHECKSUM_URL = "checksums.json";

  const shaEl = document.getElementById("asset-sha");
  const nameEl = document.getElementById("asset-name");
  const btn = document.getElementById("download-btn");

  if (nameEl) nameEl.textContent = ASSET;
  if (btn) btn.href = RELEASE_ASSET_URL;

  fetch(CHECKSUM_URL, { cache: "no-store" })
    .then((r) => (r.ok ? r.json() : null))
    .then((data) => {
      if (!shaEl) return;
      const hash = data && (data.sha256 || data.SHA256);
      shaEl.textContent = hash ? String(hash) : "see release notes";
      if (hash) shaEl.title = hash;
    })
    .catch(() => {
      if (shaEl) shaEl.textContent = "see release notes";
    });
})();
