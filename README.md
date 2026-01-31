## Vidscribe — Gemini-powered Video Transcriptions

Vidscribe is a CLI tool that uses Google Gemini and `ffmpeg` to transcribe videos and burn subtitles into a new `transcribed_<original>` file.

### What you’ll need

1. **Install ffmpeg** (required for video processing)

   **macOS**
   ```bash
   brew install ffmpeg
   # If you need libass for subtitles:
   brew install ffmpeg-full && brew link --overwrite ffmpeg-full
   ```

   **Linux (Ubuntu/Debian)**
   ```bash
   sudo apt install ffmpeg
   ```

   **Windows**
   ```powershell
   winget install ffmpeg
   ```

2. **Google Gemini API Key**
   Store your key in the `GOOGLE_API_KEY` environment variable.

   **Set `GOOGLE_API_KEY` before running**

   macOS/Linux:
   ```bash
   export GOOGLE_API_KEY="your_api_key_here"
   ```

   Windows PowerShell:
   ```powershell
   $Env:GOOGLE_API_KEY="your_api_key_here"
   ```

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
