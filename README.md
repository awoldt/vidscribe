## Vidscribe — Gemini-powered Video Transcriptions

Vidscribe is a simple CLI that transcribes a single video or every supported video inside a folder using Google Gemini. It extracts the audio via `ffmpeg`, asks Gemini to break it into segments, writes an SRT file, and then uses `ffmpeg` again to burn the subtitles back onto the original video so you get a ready-to-watch `transcribed_<original>` file.

### What you’ll need

- **ffmpeg installed and on your PATH**  
  - macOS: `brew install ffmpeg`
    - *Note: If you run into issues burning subtitles, you may need a version with `libass`. Use `brew install ffmpeg-full` and link it: `brew unlink ffmpeg && brew link --overwrite ffmpeg-full`*
  - Linux (Ubuntu/Debian): `sudo apt install ffmpeg`  
  - Windows: `winget install ffmpeg`  
- **A Google Gemini API key** stored in the `GOOGLE_API_KEY` environment variable. Without it, the tool cannot call the Gemini transcription service.

### Supported inputs

- Video files: `.mp4`, `.mov`, `.mkv`, `.webm`, `.avi`  
- Directories that contain any mix of the above video formats. Vidscribe skips non-video files automatically.

### Building & running

```
go build -o vidscribe
./vidscribe --input path/to/video.mp4
```

Or point at a directory to transcribe everything inside:

```
./vidscribe --input path/to/video-directory
```

### Environment reminders

- Set `GOOGLE_API_KEY` before running. Example (macOS/Linux):
  ```
  export GOOGLE_API_KEY="your_api_key_here"
  ```
  On Windows PowerShell:
  ```
  $Env:GOOGLE_API_KEY="your_api_key_here"
  ```
- Ensure `ffmpeg` works from your terminal (`ffmpeg -version`).
