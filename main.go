package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
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
	/*
		transcribes an entire directory.

		uses slightly different logic than single-file transcription,
		so this gets its own function.
	*/

	// read all nested files/dirs
	// remove all non video files
	var validFiles []string
	err := filepath.WalkDir(inputDirPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		// make sure a valid file
		if !slices.Contains(validVideoFormats, strings.ToLower(filepath.Ext(path))) {
			return nil
		}
		validFiles = append(validFiles, path)
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to traverse directory and its subdirectories")
	}

	startTime := time.Now()

	// create a tmp folder to place all files while program is running
	tempDirPath, err := os.MkdirTemp("", "transcribe-")
	if err != nil {
		return errors.New("error while creating temp folder")
	}
	defer os.RemoveAll(tempDirPath) // clean up the tmp files when program done

	var errs []error
	var mu sync.RWMutex
	success := 0
	numOfVids := len(validFiles)

	spinner := spinner.New(spinner.CharSets[2], 100*time.Millisecond)
	spinner.Prefix = fmt.Sprintf("Transcoding video(s) %v of %v... ", success, numOfVids)
	spinner.Start()

	// loop through entire directory and transcribe each video
	for _, fullpath := range validFiles {
		// convert video to mp3
		outputAudioPath, err := VideoToMp3(tempDirPath, fullpath)
		if err != nil {
			mu.Lock()
			errs = append(errs, err)
			mu.Unlock()
			continue
		}

		structuredResponse, err := TranscribeVideo(ctx, apiKey, c, outputAudioPath)
		if err != nil {
			mu.Lock()
			errs = append(errs, fmt.Errorf("%v\nerror while transcribing video", err.Error()))
			mu.Unlock()
			continue
		}

		filenameNoExt := strings.TrimSuffix(filepath.Base(fullpath), filepath.Ext(fullpath))
		strFilePath, err := GenerateSrtFile(&structuredResponse, filenameNoExt, tempDirPath)
		if err != nil {
			mu.Lock()
			errs = append(errs, fmt.Errorf("%v\nerror while saving srt file", err.Error()))
			mu.Unlock()
			continue
		}

		// now that we have the srt file, get ffmpeg to add subtitles
		// to the original video file
		tempVideoPath, err := ApplySubtitles(tempDirPath, fullpath, strFilePath)
		if err != nil {
			mu.Lock()
			errs = append(errs, err)
			mu.Unlock()
			continue
		}

		filename := filepath.Base(fullpath)
		// move the final file from tmp directory to root
		err = os.Rename(tempVideoPath, "./transcribed_"+filename)
		if err != nil {
			mu.Lock()
			errs = append(errs, err)
			mu.Unlock()
			continue
		}

		success++
		spinner.Prefix = fmt.Sprintf(
			"Transcoding video(s) %d of %d... ",
			success,
			numOfVids,
		)
	}

	spinner.Stop() // make sure to stop spinner before printing final message

	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Println(e.Error())
		}
	}

	fmt.Printf("Processed %d videos successfully; %d failed in %.2f seconds\n", success, len(errs), time.Since(startTime).Seconds())

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
	err = os.Rename(tempVideoPath, "transcribed_"+filepath.Base(inputPath))
	if err != nil {
		return err
	}

	spinner.FinalMSG = fmt.Sprintf("Transcription completed in %.2f seconds\n", time.Since(startTime).Seconds())
	return nil
}
