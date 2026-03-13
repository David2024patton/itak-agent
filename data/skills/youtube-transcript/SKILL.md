---
name: youtube-transcript
description: Transcribe YouTube videos to text using yt-dlp. Use when user shares a YouTube link and wants the content extracted, or when researching video content for documentation.
---

# YouTube Transcript Extraction

Extract captions and subtitles from YouTube videos without audio processing.

## Requirements
- `yt-dlp` CLI installed (`pip install yt-dlp` or `winget install yt-dlp`)
- No ffmpeg needed - works with VTT format directly

## Quick Usage

### Download subtitles (auto-generated)
```powershell
yt-dlp --write-auto-sub --sub-lang en --skip-download -o "<output_name>" "<YouTube-URL>"
```
This creates a `.en.vtt` file.

### Download manual subtitles (if available)
```powershell
yt-dlp --write-sub --sub-lang en --skip-download -o "<output_name>" "<YouTube-URL>"
```

### List available subtitle languages
```powershell
yt-dlp --list-subs "<YouTube-URL>"
```

## VTT to Plain Text Conversion

After downloading the `.vtt` file, strip timestamps, tags, and metadata:
```powershell
$raw = Get-Content "<file>.en.vtt" -Raw
$clean = $raw -replace '<[^>]+>', ''
$lines = $clean -split "`r?`n"
$result = $lines | Where-Object {
    $_ -notmatch '^\s*$' -and
    $_ -notmatch '^\d{2}:\d{2}' -and
    $_ -notmatch '^WEBVTT' -and
    $_ -notmatch '^Kind:' -and
    $_ -notmatch '^Language:' -and
    $_ -notmatch 'align:'
} | Select-Object -Unique
$result | Set-Content "transcript.txt"
```

## Triggers
Activate this skill when the user:
- Shares a YouTube URL and asks to transcribe it
- Wants to know "what does this video say"
- Needs video content for documentation or research
- Asks to summarize a YouTube video

## Output
- Save transcript to project directory or `/tmp/` for one-off requests
- Provide a summary if the user asks for one
- Note: Auto-generated subtitles may have accuracy issues; manual subs are more reliable when available
