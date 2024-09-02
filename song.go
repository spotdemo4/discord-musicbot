package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gedzeppelin/dca"
)

type song struct {
	FileName     string
	Url          string
	GuildID      string
	Name         *string
	FullFileName *string
	Buffer       *[][]byte
}

// download downloads the song
func (s *song) download() error {
	cmd := exec.Command("yt-dlp", "-x", "--audio-format", "opus", "-o", fmt.Sprintf("%s.%%(ext)s", s.FileName), s.Url)

	if err := cmd.Run(); err != nil {
		return err
	}

	// Find video file
	if err := s.find(); err != nil {
		return err
	}

	// Get the name of the song
	cmd = exec.Command("yt-dlp", "--print", "fulltitle", s.Url)

	output, err := cmd.Output()
	if err != nil {
		return err
	}

	// Set the name of the song
	name := strings.ReplaceAll(string(output), "\n", "")
	if name != "" {
		s.Name = &name
	} else {
		s.Name = &s.Url
	}

	return nil
}

// find finds the video file
func (s *song) find() error {
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if strings.HasPrefix(info.Name(), s.FileName) {
			fullName := info.Name()
			s.FullFileName = &fullName
		}

		return nil
	})
	if err != nil {
		return err
	}
	if s.FullFileName == nil {
		return errors.New("could not find song file")
	}

	return nil
}

// Converts the downloaded song to a DCA file
func (s *song) convert() error {
	options := dca.StdEncodeOptions
	options.RawOutput = true
	options.Bitrate = 128

	// Encoding a file and save it to disk
	encodeSession, err := dca.EncodeFile(*s.FullFileName, options)
	if err != nil {
		return err
	}
	// Make sure everything is cleaned up, that for example the encoding process if any issues happened isnt lingering around
	defer encodeSession.Cleanup()

	// Create a file with the same name as the original file, but with the DCA file extension
	output, err := os.Create(fmt.Sprintf("%s.dca", s.FileName))
	if err != nil {
		return err
	}

	// Copy encoded data to the file
	_, err = io.Copy(output, encodeSession)
	if err != nil {
		return err
	}

	// Delete the original file
	err = os.Remove(*s.FullFileName)
	if err != nil {
		return err
	}

	// Update the full name
	newFullName := fmt.Sprintf("%s.dca", s.FileName)
	s.FullFileName = &newFullName

	return nil
}

// load attempts to load an encoded sound file into buffer
func (s *song) load() error {
	var buffer = make([][]byte, 0)

	file, err := os.Open(*s.FullFileName)
	if err != nil {
		log.Printf("Error opening dca file: %s", err)
		return err
	}

	var opuslen int16

	for {
		// Read opus frame length from dca file.
		err = binary.Read(file, binary.LittleEndian, &opuslen)

		// If this is the end of the file, just return.
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}

		if err != nil {
			log.Printf("Error reading from dca file: %s", err)
			return err
		}

		// Read encoded pcm from dca file.
		InBuf := make([]byte, opuslen)
		err = binary.Read(file, binary.LittleEndian, &InBuf)

		// Should not be any end of file errors
		if err != nil {
			log.Printf("Error reading from dca file: %s", err)
			return err
		}

		// Append encoded pcm data to the buffer.
		buffer = append(buffer, InBuf)
	}

	// Close the file
	err = file.Close()
	if err != nil {
		return err
	}

	// Set the buffer
	s.Buffer = &buffer

	// Delete the dca file
	err = os.Remove(*s.FullFileName)
	if err != nil {
		return err
	}

	return nil
}
