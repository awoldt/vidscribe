package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/urfave/cli/v3"
	"google.golang.org/genai"
)

type Schema struct {
	Language string `json:"language"`
	Segments []struct {
		Start float64 `json:"start"`
		End   float64 `json:"end"`
		Text  string  `json:"text"`
	} `json:"segments"`
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

func TranscribeVideo(ctx context.Context, apiKey string, c *cli.Command) (Schema, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		return Schema{}, fmt.Errorf("error while creating gemini client")
	}

	localAudioPath := "output.mp3"
	uploadedFile, err := client.Files.UploadFromPath(
		ctx,
		localAudioPath,
		nil,
	)
	if err != nil {
		return Schema{}, fmt.Errorf("%v\nerror while uploading audio clip to gemini", err.Error())
	}

	parts := []*genai.Part{
		genai.NewPartFromText("Generate a transcript of the audio."),
		genai.NewPartFromURI(uploadedFile.URI, uploadedFile.MIMEType),
	}
	contents := []*genai.Content{
		genai.NewContentFromParts(parts, genai.RoleUser),
	}

	model := c.String("model")
	result, err := client.Models.GenerateContent(
		ctx,
		fmt.Sprintf("gemini-3-%v-preview", model),
		contents,
		&genai.GenerateContentConfig{
			ResponseMIMEType: "application/json",
			ResponseSchema:   TranscriptSchema,
		},
	)
	if err != nil {
		return Schema{}, fmt.Errorf("%v\nerror while generating gemini response", err.Error())
	}

	var structuredResponse Schema
	err = json.Unmarshal([]byte(result.Text()), &structuredResponse)
	if err != nil {
		return Schema{}, fmt.Errorf("%v\nerror while unmarshalling gemini response into json", err.Error())
	}

	return structuredResponse, nil
}
