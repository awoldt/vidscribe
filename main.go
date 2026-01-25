package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/joho/godotenv"
	"github.com/urfave/cli/v3"
)

var validVideoFormats = []string{".mp4", ".mov", ".mkv", ".webm", ".avi"}

func main() {
	cmd := &cli.Command{
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "input",
				Usage:    "The input video to turn into audio",
				Required: true,
			},
			&cli.StringFlag{
				Name:        "model",
				Usage:       "Underlying gemini model to use",
				Value:       "flash",
				DefaultText: "flash",
				Required:    false,
				Validator: func(s string) error {
					if s != "pro" && s != "flash" {
						return fmt.Errorf("%s is not a valid model", s)
					}
					return nil
				},
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			// ensure user has ffmpeg installed
			_, err := exec.LookPath("ffmpeg")
			if err != nil {
				return fmt.Errorf(
					"ffmpeg is required but not installed.\n\n" +
						"Install it with:\n" +
						"  macOS:    brew install ffmpeg\n" +
						"  Ubuntu:   sudo apt install ffmpeg\n" +
						"  Arch:     sudo pacman -S ffmpeg\n" +
						"  Windows:  winget install ffmpeg\n",
				)
			}

			// load env variable (need gemini api key to work)
			err = godotenv.Load()
			if err != nil {
				return fmt.Errorf("error loading .env file in current directory")
			}
			apiKey := os.Getenv("GOOGLE_API_KEY")
			if apiKey == "" {
				return fmt.Errorf("GOOGLE_API_KEY is not set")
			}

			inputFile := c.String("input")
			// make sure input video is present in root directory
			if _, err := os.Stat(inputFile); err != nil {
				return fmt.Errorf("%s does not exist in current directory", inputFile)
			}
			if fileExt := filepath.Ext(inputFile); !slices.Contains(validVideoFormats, strings.ToLower(fileExt)) {
				return fmt.Errorf(
					"unsupported input file type. Supported formats: %s",
					strings.Join(validVideoFormats, ", "),
				)
			}

			spinner := spinner.New(spinner.CharSets[2], 100*time.Millisecond)
			defer spinner.Stop()
			spinner.Prefix = "Transcoding audio... "
			spinner.Start()

			startTime := time.Now()

			// create a tmp folder to place all files while program is running
			tempDirPath, err := os.MkdirTemp("", "transcribe-")
			if err != nil {
				return fmt.Errorf("error while creating temp folder")
			}
			defer os.RemoveAll(tempDirPath) // clean up the tmp files when program done

			// run ffmpeg to convert input file to mp3
			outputAudioPath := filepath.Join(tempDirPath, "audio.mp3")
			cmd := exec.Command(
				"ffmpeg",
				"-y", // this will overwrite the output video if already exists
				"-i", inputFile,
				outputAudioPath,
			)
			err = cmd.Run()
			if err != nil {
				return fmt.Errorf("%v\nerror while converting %s to audio format", err.Error(), inputFile)
			}

			spinner.Prefix = "Transcribing audio... "
			structuredResponse, err := TranscribeVideo(ctx, apiKey, c, outputAudioPath)
			if err != nil {
				return fmt.Errorf("%v\nerror while transcribing video", err.Error())
			}

			strFilePath, err := GenerateSrtFile(&structuredResponse, tempDirPath)
			if err != nil {
				return fmt.Errorf("%v\nerror while saving srt file", err.Error())
			}

			spinner.Prefix = "Adding subtitles overlay... "

			// now that we have the srt file, get ffmpeg to add subtitles
			// to the original video file
			finalVideoPath := "transcribed_" + inputFile
			tempVideoPath := filepath.Join(tempDirPath, finalVideoPath)
			cmd = exec.Command("ffmpeg",
				"-y",
				"-i", inputFile,
				"-vf",
				"subtitles="+strFilePath,
				tempVideoPath, // place in tmp folder
			)
			err = cmd.Run()
			if err != nil {
				return fmt.Errorf("%v\nerror while adding subtitles to original video", err.Error())
			}

			// now copy that video out of the tmp folder and place in root
			// SUCCESS!
			in, err := os.Open(tempVideoPath)
			if err != nil {
				return fmt.Errorf("%v\nerror while copying tmp video to root directory", err.Error())
			}
			defer in.Close()

			out, err := os.Create(finalVideoPath)
			if err != nil {
				return err
			}
			defer out.Close()

			_, err = io.Copy(out, in)
			if err != nil {
				return err
			}

			spinner.FinalMSG = fmt.Sprintf("Transcription completed in %v seconds\n", fmt.Sprintf("%.2f", time.Since(startTime).Seconds()))
			return nil
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
