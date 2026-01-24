package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/joho/godotenv"
	"google.golang.org/genai"
)

func main() {
	// first make sure input video is present
	inputVideo := "vid.MOV"
	if _, err := os.Stat(inputVideo); err != nil {
		log.Fatal("input mp4 file doesnt exist")
	}

	// run ffmpeg to conver input mp4 file to mp3
	cmd := exec.Command(
		"ffmpeg",
		"-i", inputVideo,
		"output.mp3",
	)
	err := cmd.Run()
	if err != nil {
		fmt.Println(err.Error())
		log.Fatal("FAIL")
	}

	err = godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	apiKey := os.Getenv("GOOGLE_API_KEY")

	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		log.Fatal(err)
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
		nil, // put a response schema here evenautlaly
	)

	fmt.Println(result.Text())
}
