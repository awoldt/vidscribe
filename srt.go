package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

func GenerateSrtFile(transcript *Schema) {
	// take in the gemini response and create a
	// valid formatted SRT file

	var sb strings.Builder
	for i, v := range transcript.Segments {
		// NUM
		num := strconv.Itoa(i + 1)
		sb.WriteString(num + "\n")

		// TIMESTAMP
		start := getTimestamp(v.Start)
		end := getTimestamp(v.End)
		sb.WriteString(fmt.Sprintf("%v --> %v\n", start, end))

		// TEXT
		sb.WriteString("- " + v.Text)

		///////////////////////////////
		fmt.Println(sb.String())

		sb.Reset()
	}
}

func getTimestamp(time float64) string {
	// gemini seems to return timestamps in seconds (ex: 186.4)
	// so return value would be -> 00:03:06,400

	hour := fmt.Sprintf("%02d", int(math.Floor(time/60/60)))
	min := fmt.Sprintf("%02d", int(math.Floor(time/60)))
	sec := fmt.Sprintf("%02d", int(math.Floor(time)))
	ms := fmt.Sprintf("%03d", int((time-float64(int(time)))*1000))

	return fmt.Sprintf("%v:%v:%v,%v", hour, min, sec, ms)
}
