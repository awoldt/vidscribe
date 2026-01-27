package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/briandowns/spinner"
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
				err = transcribeFile(inputPath, apiKey, ctx, c)
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
	var mu sync.RWMutex
	success := 0
	numOfVids := len(files)

	spinner := spinner.New(spinner.CharSets[2], 100*time.Millisecond)
	defer spinner.Stop()
	spinner.Prefix = fmt.Sprintf("Transcoding video(s) %v of %v... ", success, numOfVids)
	spinner.Start()

	for _, file := range files {
		filename := file.Name()
		fullPath := filepath.Join(inputDirPath, filename)
		if fileExt := filepath.Ext(fullPath); !slices.Contains(validVideoFormats, strings.ToLower(fileExt)) {
			continue
		}

		wg.Go(func() {
			// convert video to mp3
			outputAudioPath, err := VideoToMp3(tempDirPath, fullPath)
			if err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
				return
			}

			structuredResponse, err := TranscribeVideo(ctx, apiKey, c, outputAudioPath)
			if err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("%v\nerror while transcribing video", err.Error()))
				mu.Unlock()
				return
			}

			filenameNoExt := strings.TrimSuffix(filepath.Base(fullPath), filepath.Ext(fullPath))
			strFilePath, err := GenerateSrtFile(&structuredResponse, filenameNoExt, tempDirPath)
			if err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("%v\nerror while saving srt file", err.Error()))
				mu.Unlock()
				return
			}

			// now that we have the srt file, get ffmpeg to add subtitles
			// to the original video file
			tempVideoPath, err := ApplySubtitles(tempDirPath, fullPath, strFilePath)
			if err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
				return
			}

			// move the final file from tmp directory to root
			err = os.Rename(tempVideoPath, "./transcribed_"+filename)
			if err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
				return
			}

			mu.Lock()
			success++
			spinner.Prefix = fmt.Sprintf("Transcoding video(s) %v of %v... ", success, numOfVids)
			mu.Unlock()
		})
	}
	wg.Wait()
	spinner.Stop()

	if len(errs) > 0 {
		mu.RLock()
		for _, e := range errs {
			fmt.Println(e.Error())
		}
		mu.RUnlock()
	}

	fmt.Printf("finished %v videos wiht %v errors", success, len(errs))

	return nil
}

func transcribeFile(inputPath, apiKey string, ctx context.Context, c *cli.Command) error {
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

	// convert video to mp3
	outputAudioPath, err := VideoToMp3(tempDirPath, inputPath)
	if err != nil {
		return err
	}

	spinner.Prefix = "Transcribing audio... "
	structuredResponse, err := TranscribeVideo(ctx, apiKey, c, outputAudioPath)
	if err != nil {
		return fmt.Errorf("%v\nerror while transcribing video", err.Error())
	}

	filename := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	strFilePath, err := GenerateSrtFile(&structuredResponse, filename, tempDirPath)
	if err != nil {
		return fmt.Errorf("%v\nerror while saving srt file", err.Error())
	}

	spinner.Prefix = "Adding subtitles overlay... "

	// now that we have the srt file, get ffmpeg to add subtitles
	// to the original video file
	tempVideoPath, err := ApplySubtitles(tempDirPath, inputPath, strFilePath)
	if err != nil {
		return err
	}

	// move the final file from tmp directory to root
	err = os.Rename(tempVideoPath, "./transcribed_"+inputPath)
	if err != nil {
		return err
	}

	spinner.FinalMSG = fmt.Sprintf("Transcription completed in %v seconds\n", fmt.Sprintf("%.2f", time.Since(startTime).Seconds()))
	return nil
}
