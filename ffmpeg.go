package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

func VideoToMp3(tempDirPath, fileinputPath string) (string, error) {
	// takes a video input and exports the mp3 audio
	filename := filepath.Base(fileinputPath)
	outputAudioPath := filepath.Join(tempDirPath, filename+"_audio.mp3")

	cmd := exec.Command(
		"ffmpeg",
		"-y", // this will overwrite the output video if already exists
		"-i", fileinputPath,
		outputAudioPath,
	)

	// better performance
	cmd.Stdout = nil
	cmd.Stderr = nil

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("%v\nerror while converting %s to audio format", err.Error(), fileinputPath)
	}

	return outputAudioPath, nil
}

func ApplySubtitles(tempDirPath, fileinputPath, strFilePath string) (string, error) {
	// takes in an srt file and applies the subtitle text over the original video
	finalVideoPath := "transcribed_" + filepath.Base(fileinputPath)
	tempVideoPath := filepath.Join(tempDirPath, finalVideoPath)

	// ffmpeg filter flag path is a pain in the ass, escape this stuff
	escaped := strings.ReplaceAll(strFilePath, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, ":", "\\:")

	cmd := exec.Command("ffmpeg",
		"-y",
		"-i", fileinputPath,
		"-vf", fmt.Sprintf("subtitles='%s'", escaped),
		tempVideoPath, // place in tmp folder
	)
	// better performance
	cmd.Stdout = nil
	cmd.Stderr = nil

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("%v\nerror while adding subtitles to original video", err.Error())
	}

	return tempVideoPath, nil
}
