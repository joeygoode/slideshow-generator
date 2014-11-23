package main

import (
	"bufio"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"io/ioutil"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joeygoode/wav"
)

const directoryName = "CS4500-code-walk"

type audioType int

const (
	MP3 audioType = iota
	WAV
)

var zeroTime time.Time

func init() {
	var err error
	zeroTime, err = time.Parse("15:04:05", "00:00:00")
	if err != nil {
		panic(err)
	}
}

func main() {
	if err := process(); err != nil {
		log.Fatal(err)
	}
}

func process() (err error) {

	// Verify arguments
	// First arg is name of program
	if len(os.Args) != 2 {
		return fmt.Errorf("expected 1 arg but got %d", len(os.Args)-1)
	}
	tarballPath := os.Args[1]
	base := filepath.Base(tarballPath)
	if base == "." {
		return fmt.Errorf("expected path to lead to a .tar.gz archive")
	}
	splitBase := strings.Split(base, ".")
	if splitBase[len(splitBase)-2] != "tar" ||
		splitBase[len(splitBase)-1] != "gz" {
		return fmt.Errorf("expected path to lead to a .tar.gz archive")
	}
	if _, err = os.Stat(tarballPath); os.IsNotExist(err) {
		return fmt.Errorf("no such file: %s", tarballPath)
	}

	// Setup Directories
	startDir, err := os.Getwd()
	if err != nil {
		return err
	}
	workDir := filepath.Join(os.TempDir(), directoryName)
	err = os.Mkdir(workDir, 0770)
	if err != nil {
		return err
	}
	defer func() {
		err2 := os.RemoveAll(workDir)
		if err2 != nil && err == nil {
			err = err2
		}
	}()
	err = os.Chdir(workDir)
	if err != nil {
		return err
	}

	// Unpack tarball
	err = exec.Command("tar", "-xzf", filepath.Join(startDir, tarballPath)).Run()
	if err != nil {
		return fmt.Errorf("tar returned error: %s", err)
	}

	// Find tarball directory
	infos, err := ioutil.ReadDir(workDir)
	if err != nil {
		return err
	}
	if len(infos) != 1 || !infos[0].IsDir() {
		printDir(workDir)
		return fmt.Errorf("can't find tarball output")
	}
	tarBase := strings.Join(splitBase[0:len(splitBase)-2], ".")
	tarDir := filepath.Join(workDir, infos[0].Name())

	// Verify tarball contents
	infos, err = ioutil.ReadDir(tarDir)
	if err != nil {
		return err
	}
	var aType audioType
	audioFoundError := fmt.Errorf("no audio found")
	imageFoundError := fmt.Errorf("no img directory found")
	timecodesFoundError := fmt.Errorf("no timecodes found")
	for _, info := range infos {
		switch info.Name() {
		case "audio.mp3":
			aType = MP3
			audioFoundError = nil
		case "audio.wav":
			aType = WAV
			audioFoundError = nil
		case "img":
			if !info.IsDir() {
				imageFoundError = fmt.Errorf("expected img to be a directory")
			} else {
				imageFoundError = nil
			}
		case "timecodes.txt":
			timecodesFoundError = nil
		}
	}
	if audioFoundError != nil {
		printDir(tarDir)
		return audioFoundError
	}
	if imageFoundError != nil {
		printDir(tarDir)
		return imageFoundError
	}
	if timecodesFoundError != nil {
		printDir(tarDir)
		return timecodesFoundError
	}
	// Convert audio
	if aType == MP3 {
		err = exec.Command("lame", "--decode", filepath.Join(tarDir, "audio.mp3"), filepath.Join(tarDir, "audio.wav")).Run()
		if err != nil {
			return fmt.Errorf("lame returned error: %s", err)
		}
	}

	// Verify images
	imgDir := filepath.Join(tarDir, "img")
	infos, err = ioutil.ReadDir(imgDir)
	if err != nil {
		return err
	}
	digitCount := 0
	currentImg := 0
	var rectSize *image.Point
	for _, info := range infos {
		if name := info.Name(); name[0:3] == "img" && filepath.Ext(name) == ".jpg" {
			digits := name[3 : len(name)-4]
			if digitCount == 0 {
				digitCount = len(digits)
			}
			if len(digits) != digitCount {
				printDir(imgDir)
				return fmt.Errorf("found bad image: %s", name)
			}
			imgNum, err := strconv.Atoi(digits)
			if err != nil {
				printDir(imgDir)
				return fmt.Errorf("found bad image: %s", name)
			}
			if imgNum != currentImg {
				printDir(imgDir)
				return fmt.Errorf("found image out of order: expected %d got %d in %s", currentImg, imgNum, name)
			}
			imgF, err := os.Open(filepath.Join(imgDir, name))
			if err != nil {
				return err
			}
			defer imgF.Close()
			img, err := jpeg.Decode(imgF)
			if err != nil {
				return fmt.Errorf("malformed jpeg %s: %s", name, err)
			}
			if rectSize == nil {
				size := img.Bounds().Size()
				rectSize = &size
			}
			if img.Bounds().Size() != *rectSize {
				return fmt.Errorf("image is of unexpected size: expected %+v, got %+v", *rectSize, img.Bounds().Size())
			}
			currentImg++
		}
	}

	// Verify and import timecodes
	timeCodesF, err := os.Open(filepath.Join(tarDir, "timecodes.txt"))
	if err != nil {
		return err
	}
	defer timeCodesF.Close()
	bufTimeCodesR := bufio.NewReader(timeCodesF)
	currentTimeCode := 0
	durations := []time.Duration{}
	var totalDuration time.Duration = 0
	var line []byte
	var loopErr error
	for line, _, loopErr = bufTimeCodesR.ReadLine(); loopErr == nil; line, _, loopErr = bufTimeCodesR.ReadLine() {
		s := strings.Split(string(line), ".")
		msec := 0
		switch len(s) {
		case 1:
		case 2:
			msec, err = strconv.Atoi(s[1])
			if err != nil {
				return fmt.Errorf("malformatted timecode %s: %s", string(line), err)
			}
		default:
			return fmt.Errorf("malformatted timecode %s: expected format hh:mm:ss[.xxx]", string(line))
		}
		t, err := time.Parse("15:04:05", s[0])
		if err != nil {
			return err
		}
		endTime := -zeroTime.Sub(t)
		endTime += time.Duration(msec) * time.Millisecond
		if endTime <= totalDuration {
			return fmt.Errorf("got timecode out of order: %s (%v) but only %v time has passed", string(line), endTime, totalDuration)
		}
		duration := endTime - totalDuration
		durations = append(durations, duration)
		totalDuration += duration
		currentTimeCode++
	}
	if loopErr != io.EOF {
		if err == nil {
			panic("shouldn't have exited loop!!")
		}
		return err
	}
	timeCodesF.Close()

	// Verify image and timecode counts
	if currentTimeCode+1 != currentImg {
		return fmt.Errorf("mismatched timecode (%d) and image counts (%d)", currentTimeCode, currentImg)
	}

	// Get total duration from audio
	audioF, err := os.Open(filepath.Join(tarDir, "audio.wav"))
	if err != nil {
		return err
	}
	defer audioF.Close()
	audioStat, err := audioF.Stat()
	if err != nil {
		return err
	}
	wavAudio, err := wav.NewWavReader(audioF, audioStat.Size())
	if err != nil {
		return err
	}
	endTime := wavAudio.GetDuration()
	if endTime <= totalDuration {
		return fmt.Errorf("audio ends before last image is displayed")
	}
	durations = append(durations, endTime-totalDuration)
	wavAudio = nil
	audioF.Close()

	// Create video from images and durations
	vidDir := filepath.Join(tarDir, "vid")
	err = os.Mkdir(vidDir, 0770)
	if err != nil {
		return err
	}
	for i := 0; i < currentImg; i++ {
		duration := durations[i]
		hours := time.Duration(math.Floor(duration.Hours()))
		duration -= hours * time.Hour
		minutes := time.Duration(math.Floor(duration.Minutes()))
		duration -= minutes * time.Minute
		seconds := time.Duration(math.Floor(duration.Seconds()))
		duration -= seconds * time.Second
		ms := duration / time.Millisecond
		cmd := exec.Command("ffmpeg",
			"-loop", "1",
			"-i", filepath.Join(imgDir, fmt.Sprintf("img%0*d.jpg", digitCount, i)),
			"-c:v", "libx264",
			"-pix_fmt", "yuv420p",
			"-t", fmt.Sprintf("%02d:%02d:%02d.%03d", hours, minutes, seconds, ms),
			filepath.Join(vidDir, fmt.Sprintf("vid%0*d.mp4", digitCount, i)))
		err := cmd.Run()
		if err != nil {
			return fmt.Errorf("ffmpeg returned error: %s", err)
		}
	}

	// Concatenate videos
	concatInstrPath := filepath.Join(vidDir, "list.txt")
	concatInstr, err := os.Create(concatInstrPath)
	if err != nil {
		return err
	}
	defer concatInstr.Close()
	for i := 0; i < currentImg; i++ {
		fmt.Fprintf(concatInstr, "file %s\n",
			filepath.Join(vidDir, fmt.Sprintf("vid%0*d.mp4", digitCount, i)))
	}
	concatInstr.Close()

	cmd := exec.Command("ffmpeg",
		"-f", "concat",
		"-i", concatInstrPath,
		"-c", "copy",
		filepath.Join(vidDir, "vid.mp4"))
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("ffmpeg returned error: %s", err)
	}

	// Encode audio as mp3
	if _, err := os.Stat(filepath.Join(tarDir, "audio.mp3")); os.IsNotExist(err) {
		err = exec.Command("lame",
			"-b", "192",
			filepath.Join(tarDir, "audio.wav"),
			filepath.Join(tarDir, "audio.mp3")).Run()
		if err != nil {
			return fmt.Errorf("lame returned error")
		}
	}

	// Add audio
	cmd = exec.Command("ffmpeg",
		"-i", filepath.Join(vidDir, "vid.mp4"),
		"-i", filepath.Join(tarDir, "audio.mp3"),
		"-map", "0:v",
		"-map", "1:a",
		"-codec", "copy",
		"-shortest", filepath.Join(startDir, fmt.Sprintf("%s.%s", tarBase, "mp4")))
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("ffmpeg returned error: %s", err)
	}
	return nil
}

func printDir(dir string) {
	infos, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Dumping directory contents:")
	for _, info := range infos {
		fmt.Println(info.Name())
	}
}
