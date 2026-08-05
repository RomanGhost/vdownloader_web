"use strict";

const form = document.getElementById("url-form");
const urlInput = document.getElementById("url-input");
const statusEl = document.getElementById("status");
const titleEl = document.getElementById("title");
const formatsEl = document.getElementById("formats");

let currentURL = "";
let currentTitle = "";
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

function isAudioOnly(f) {
  return f.have_audio && !f.have_video;
}

function isVideoOnly(f) {
  return f.have_video && !f.have_audio;
}

function qualityLabel(f) {
  return f.format_note || f.resolution || f.format_id;
}

function formatLabel(f) {
  const parts = [f.format_note || f.resolution || "audio"];
  if (f.ext) parts.push(f.ext);
  if (f.filesize > 0) {
    parts.push((f.filesize / (1024 * 1024)).toFixed(1) + " MiB");
  } else if (f.tbr > 0) {
    parts.push(Math.round(f.tbr) + " kbps");
  }
  return parts.join(" • ");
}

function formatToJobRequest(url, title, f) {
  const req = {
    url,
    title,
    quality_label: qualityLabel(f),
    audio_only: isAudioOnly(f),
  };
  if (isVideoOnly(f)) {
    req.format_arg = f.format_id + "+bestaudio";
    req.output_format = "mp4";
  } else {
    req.format_arg = f.format_id;
  }
  return req;
}

// ── UI ───────────────────────────────────────────────────────────────────

function renderFormats(formats) {
  formatsEl.innerHTML = "";
  formats.forEach((f) => {
    const li = document.createElement("li");
    const btn = document.createElement("button");
    btn.type = "button";
    btn.textContent = formatLabel(f);
    btn.addEventListener("click", () => startDownload(f));
    li.appendChild(btn);
    formatsEl.appendChild(li);
  });
  formatsEl.hidden = formats.length === 0;
}

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
  titleEl.textContent = currentTitle;
  titleEl.hidden = false;

  setStatus("Select a format:", false);
  renderFormats(data.formats || []);
});

async function startDownload(format) {
  formatsEl.hidden = true;
  setStatus("Queuing download…", false);

  const body = formatToJobRequest(currentURL, currentTitle, format);

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

  setStatus(`Downloading (job #${data.job_id})…`, false);
  pollJob(data.job_id);
}

function pollJob(jobID) {
  const poll = async () => {
    let resp;
    try {
      resp = await fetch("/api/jobs/" + jobID);
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
