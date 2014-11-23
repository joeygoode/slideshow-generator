# CS4500-code-walk
===

Little-ish program I wrote to convert audio + images into a video
slideshow.  Sample input is provided in sample.tar.gz.

## Usage
---

go run main.go in-file

## Input format
---

One tarball (.tar.gz) containing:
* An MP3 file named "audio.mp3" OR a WAV file named "audio.wav"
* A folder named "img" containing:
  * A sequence of JPEG images with names matching img[0-9]*.jpg
  * The sequence must start at zero
* A plaintext file named timecodes.txt
  * Each line contains one time code formatted as "hh:mm:ss[.xxx]"
  * No trailing spaces or newlines

## Output format

One H264 MP4 video file will be written to the directory where the
input was located