"use strict";

const form = document.getElementById("url-form");
const urlInput = document.getElementById("url-input");
const statusEl = document.getElementById("status");
const titleEl = document.getElementById("title");
const formatsEl = document.getElementById("formats");

let currentURL = "";
let currentTitle = "";
let currentDuration = 0;
let videoHeights = [];
let audioFormats = [];
let pollTimer = null;

function setStatus(text, isError) {
  statusEl.hidden = !text;
  statusEl.textContent = text || "";
  statusEl.classList.toggle("error", Boolean(isError));
}

function stopPolling() {
  if (pollTimer) {
    clearTimeout(pollTimer);
    pollTimer = null;
  }
}

// ── format helpers (mirrors vdownloader_telegram/internal/bot/presets.go) ──

function heightLabel(height) {
  return height === 2160 ? "4K (2160p)" : height + "p";
}

// ── step rendering ──────────────────────────────────────────────────────────

function renderButtons(items) {
  formatsEl.innerHTML = "";
  items.forEach(({ label, onClick }) => {
    const li = document.createElement("li");
    const btn = document.createElement("button");
    btn.type = "button";
    btn.textContent = label;
    btn.addEventListener("click", onClick);
    li.appendChild(btn);
    formatsEl.appendChild(li);
  });
  formatsEl.hidden = items.length === 0;
}

// Step 1: pick a video quality tier, or go to the audio-only branch.
function renderQualityStep() {
  setStatus("Select a quality:", false);
  const items = videoHeights.map((h) => ({
    label: heightLabel(h),
    onClick: () => renderVideoAudioStep(h),
  }));
  items.push({ label: "🎵 Audio only", onClick: renderAudioFormatStep });
  renderButtons(items);
}

// Step 2 (video branch): with or without an audio track.
function renderVideoAudioStep(height) {
  setStatus(`${heightLabel(height)} — with or without audio?`, false);
  renderButtons([
    { label: "🔊 With audio", onClick: () => startDownload({ kind: "video", height, withAudio: true }) },
    { label: "🔇 Without audio", onClick: () => startDownload({ kind: "video", height, withAudio: false }) },
    { label: "← Back", onClick: renderQualityStep },
  ]);
}

// Step 2 (audio branch): target codec, mp3 first as the default.
function renderAudioFormatStep() {
  setStatus("Select an audio format:", false);
  const items = audioFormats.map((f, i) => ({
    label: f.toUpperCase() + (i === 0 ? " (default)" : ""),
    onClick: () => startDownload({ kind: "audio", audioFormat: f }),
  }));
  items.push({ label: "← Back", onClick: renderQualityStep });
  renderButtons(items);
}

// ── form submit / download ──────────────────────────────────────────────────

form.addEventListener("submit", async (e) => {
  e.preventDefault();
  stopPolling();
  formatsEl.hidden = true;
  titleEl.hidden = true;

  const url = urlInput.value.trim();
  if (!url) return;
  currentURL = url;

  setStatus("Fetching video info…", false);

  let resp;
  try {
    resp = await fetch("/api/formats?url=" + encodeURIComponent(url));
  } catch (err) {
    setStatus("Network error: " + err.message, true);
    return;
  }

  let data;
  try {
    data = await resp.json();
  } catch {
    setStatus("Unexpected response from server", true);
    return;
  }

  if (!resp.ok || data.error) {
    setStatus("Failed to get video info: " + (data.error || resp.statusText), true);
    return;
  }

  currentTitle = data.title || url;
  currentDuration = data.duration || 0;
  videoHeights = data.video_heights || [];
  audioFormats = data.audio_formats || [];

  titleEl.textContent = currentTitle;
  titleEl.hidden = false;

  renderQualityStep();
});

async function startDownload({ kind, height, withAudio, audioFormat }) {
  formatsEl.hidden = true;
  setStatus("Queuing download…", false);

  const body = { url: currentURL, title: currentTitle, duration: currentDuration, kind };
  if (kind === "video") {
    body.height = height;
    body.with_audio = withAudio;
  } else {
    body.audio_format = audioFormat;
  }

  let resp;
  try {
    resp = await fetch("/api/jobs", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
  } catch (err) {
    setStatus("Network error: " + err.message, true);
    return;
  }

  let data;
  try {
    data = await resp.json();
  } catch {
    setStatus("Unexpected response from server", true);
    return;
  }

  if (!resp.ok || data.error) {
    setStatus("Failed to queue download: " + (data.error || resp.statusText), true);
    return;
  }

  setStatus("Downloading…", false);
  pollJob(data.file_id);
}

function pollJob(fileID) {
  const poll = async () => {
    let resp;
    try {
      resp = await fetch("/api/jobs/" + fileID);
    } catch (err) {
      setStatus("Network error: " + err.message, true);
      pollTimer = setTimeout(poll, 2000);
      return;
    }

    if (!resp.ok) {
      setStatus("Failed to fetch job status: " + resp.statusText, true);
      pollTimer = setTimeout(poll, 2000);
      return;
    }

    const job = await resp.json();

    if (job.status === "pending") {
      pollTimer = setTimeout(poll, 2000);
      return;
    }

    if (job.status === "failed") {
      setStatus("Download failed: " + (job.error || "unknown error"), true);
      return;
    }

    // ready
    statusEl.hidden = false;
    statusEl.classList.remove("error");
    statusEl.textContent = "";
    const link = document.createElement("a");
    link.href = job.download_url;
    link.setAttribute("download", "");
    link.textContent = "⬇️ Download file";
    statusEl.appendChild(link);
  };

  poll();
}
