package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/joho/godotenv"
	"github.com/urfave/cli/v3"
	"google.golang.org/genai"
)

type Schema struct {
	Language string `json:"language"`
	Segments []struct {
		Start int    `json:"start"`
		End   int    `json:"end"`
		Text  string `json:"text"`
	} `json:"segments"`
}

func main() {
	cmd := &cli.Command{
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "input",
				Usage:    "The input video to turn into audio",
				Required: true,
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
				return fmt.Errorf(
					"GOOGLE_API_KEY is not set.\n\n" +
						"Create an API key:\n" +
						"  1. Go to https://ai.google.dev\n" +
						"  2. Create a project (or select an existing one)\n" +
						"  3. Generate an API key\n\n",
				)
			}

			inputFile := c.String("input")

			// make sure input video is present
			if _, err := os.Stat(inputFile); err != nil {
				return fmt.Errorf("%s does not exist in current directory", inputFile)
			}

			// run ffmpeg to conver input mp4 file to mp3
			cmd := exec.Command(
				"ffmpeg",
				"-i", inputFile,
				"output.mp3",
			)
			err = cmd.Run()
			if err != nil {
				return fmt.Errorf("%v\nerror while converting %s to audio format", err.Error(), inputFile)
			}

			client, err := genai.NewClient(ctx, &genai.ClientConfig{
				APIKey: apiKey,
			})
			if err != nil {
				return fmt.Errorf("error while creating gemini client")
			}

			localAudioPath := "output.mp3"
			uploadedFile, _ := client.Files.UploadFromPath(
				ctx,
				localAudioPath,
				nil,
			)

			parts := []*genai.Part{
				genai.NewPartFromText("Generate a transcript of the speech."),
				genai.NewPartFromURI(uploadedFile.URI, uploadedFile.MIMEType),
			}
			contents := []*genai.Content{
				genai.NewContentFromParts(parts, genai.RoleUser),
			}

			result, _ := client.Models.GenerateContent(
				ctx,
				"gemini-3-flash-preview",
				contents,
				&genai.GenerateContentConfig{
					ResponseMIMEType: "application/json",
					ResponseSchema:   TranscriptSchema,
				},
			)

			fmt.Println(result.Text())

			var structuredResponse Schema
			err = json.Unmarshal([]byte(result.Text()), &structuredResponse)
			if err != nil {
				return fmt.Errorf("error while unmarshalling gemini response into json")
			}

			fmt.Println(structuredResponse)

			return nil // END OF PROGRAM!
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}

var TranscriptSchema = &genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"language": {
			Type: genai.TypeString,
		},
		"segments": {
			Type: genai.TypeArray,
			Items: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"start": {Type: genai.TypeNumber},
					"end":   {Type: genai.TypeNumber},
					"text":  {Type: genai.TypeString},
				},
				Required: []string{"start", "end", "text"},
			},
		},
	},
	Required: []string{"language", "segments"},
}
