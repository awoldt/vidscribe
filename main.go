package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
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
				Usage:    "Path to a video file or a directory containing video files to transcribe",
				Required: true,
			},
			&cli.StringFlag{
				Name:        "model",
				Usage:       "Underlying Google Gemini model to use for video transcription",
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

			inputPath := c.String("input")
			info, err := os.Stat(inputPath)
			if err != nil {
				return errors.New("unable to read input path: " + inputPath)
			}
			if info.IsDir() {
				// many videos
				err := transcribeDir(inputPath, apiKey, ctx, c)
				if err != nil {
					return err
				}
			} else {
				// single video
				err = transcribeFile(inputPath, "./", apiKey, ctx, c)
				if err != nil {
					return err
				}
			}
			return nil
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}

func transcribeDir(inputDirPath, apiKey string, ctx context.Context, c *cli.Command) error {
	// transcribes an entire directory
	// slightly different logic from single file
	// so gets its own function

	files, err := os.ReadDir(inputDirPath)
	if err != nil {
		return fmt.Errorf("%v\nthere was an error while reading the directory %v", err.Error(), inputDirPath)
	}

	// create a output folder to place all transcribed videos
	outputPath := "./output"
	os.RemoveAll(outputPath)
	err = os.Mkdir(outputPath, 0644)
	if err != nil {
		return fmt.Errorf("%v\nthere was an error while making output directory", err.Error())
	}

	success := 0
	spinner := spinner.New(spinner.CharSets[2], 100*time.Millisecond)
	defer spinner.Stop()
	spinner.Prefix = fmt.Sprintf("Transcoding video(s) %v of %v... ", success, len(files))
	spinner.Start()

	// create a tmp folder to place all files while program is running
	tempDirPath, err := os.MkdirTemp("", "transcribe-")
	if err != nil {
		return errors.New("error while creating temp folder")
	}
	defer os.RemoveAll(tempDirPath) // clean up the tmp files when program done

	// loop through entire directory and transcribe each video
	// use go routines fast af
	var wg sync.WaitGroup
	var errs []error
	for _, file := range files {
		fullPath := filepath.Join(inputDirPath, file.Name())
		if fileExt := filepath.Ext(fullPath); !slices.Contains(validVideoFormats, strings.ToLower(fileExt)) {
			continue
		}

		wg.Go(func() {
			// run ffmpeg to convert input file to mp3
			outputAudioPath := filepath.Join(tempDirPath, file.Name()+"_audio.mp3")
			cmd := exec.Command(
				"ffmpeg",
				"-y",
				"-i", fullPath,
				outputAudioPath,
			)
			err = cmd.Run()
			if err != nil {
				errs = append(errs, fmt.Errorf("%v\nerror while converting %s to audio format", err.Error(), fullPath))
				return
			}

			structuredResponse, err := TranscribeVideo(ctx, apiKey, c, outputAudioPath)
			if err != nil {
				errs = append(errs, fmt.Errorf("%v\nerror while transcribing video", err.Error()))
				return
			}

			strFilePath, err := GenerateSrtFile(&structuredResponse, tempDirPath)
			if err != nil {
				errs = append(errs, fmt.Errorf("%v\nerror while saving srt file", err.Error()))
				return
			}

			// now that we have the srt file, get ffmpeg to add subtitles
			// to the original video file
			finalVideoPath := filepath.Join(outputPath, "transcribed_"+file.Name())
			tempVideoPath := filepath.Join(tempDirPath, finalVideoPath)
			cmd = exec.Command("ffmpeg",
				"-y",
				"-i", fullPath,
				"-vf",
				"subtitles="+strFilePath,
				tempVideoPath, // place in tmp folder
			)
			err = cmd.Run()
			if err != nil {
				errs = append(errs, fmt.Errorf("%v\nerror while adding subtitles to original video", err.Error()))
				return
			}

			// now copy that video out of the tmp folder and place in root
			// SUCCESS!
			in, err := os.Open(tempVideoPath)
			if err != nil {
				errs = append(errs, fmt.Errorf("%v\nerror while copying tmp video to root directory", err.Error()))
				return
			}
			defer in.Close()

			out, err := os.Create(finalVideoPath)
			if err != nil {
				errs = append(errs, err)
				return
			}
			defer out.Close()

			_, err = io.Copy(out, in)
			if err != nil {
				errs = append(errs, err)
				return
			}

			success++
			spinner.Prefix = fmt.Sprintf("Transcoding video(s) %v of %v... ", success, len(files))
		})
	}
	wg.Wait()
	spinner.Stop()

	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Println(e.Error())
		}
		return errors.New("there were errors")
	}

	fmt.Printf("finished %v videos wiht %v errors", success, len(errs))

	return nil
}

func transcribeFile(inputPath, outputPath, apiKey string, ctx context.Context, c *cli.Command) error {
	// make sure its a valid video format
	if fileExt := filepath.Ext(inputPath); !slices.Contains(validVideoFormats, strings.ToLower(fileExt)) {
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
		return errors.New("error while creating temp folder")
	}
	defer os.RemoveAll(tempDirPath) // clean up the tmp files when program done

	// run ffmpeg to convert input file to mp3
	outputAudioPath := filepath.Join(tempDirPath, "audio.mp3")
	cmd := exec.Command(
		"ffmpeg",
		"-y", // this will overwrite the output video if already exists
		"-i", inputPath,
		outputAudioPath,
	)
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("%v\nerror while converting %s to audio format", err.Error(), inputPath)
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
	finalVideoPath := "transcribed_" + inputPath
	tempVideoPath := filepath.Join(tempDirPath, finalVideoPath)
	cmd = exec.Command("ffmpeg",
		"-y",
		"-i", inputPath,
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
}
