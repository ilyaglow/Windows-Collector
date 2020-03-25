// Copyright (c) 2020 Alec Randazzo

package windowscollector

import (
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode"

	vbr "github.com/Go-Forensics/VBR-Parser"
	log "github.com/sirupsen/logrus"
	syscall "golang.org/x/sys/windows"
)

type handler interface {
	GetHandle(volumeLetter string) (handle *os.File, err error)
}

// VolumeHandler contains everything needed for basic collection functionality
type VolumeHandler struct {
	Handle               *os.File
	VolumeLetter         string
	Vbr                  vbr.VolumeBootRecord
	mftReader            io.Reader
	lastReadVolumeOffset int64
}

// GetHandle will get a file handle to the underlying NTFS volume. We need this in order to bypass file locks.
func (volume VolumeHandler) GetHandle(volumeLetter string) (handle *os.File, err error) {
	dwDesiredAccess := uint32(0x80000000) //0x80 FILE_READ_ATTRIBUTES
	dwShareMode := uint32(0x02 | 0x01)
	dwCreationDisposition := uint32(0x03)
	dwFlagsAndAttributes := uint32(0x00)

	volumePath, _ := syscall.UTF16PtrFromString(fmt.Sprintf("\\\\.\\%s:", volumeLetter))
	syscallHandle, err := syscall.CreateFile(volumePath, dwDesiredAccess, dwShareMode, nil, dwCreationDisposition, dwFlagsAndAttributes, 0)
	if err != nil {
		err = fmt.Errorf("getHandle() failed to get handle to volume %s: %w", volumeLetter, err)
		return
	}
	handle = os.NewFile(uintptr(syscallHandle), "")
	return
}

// GetVolumeHandler gets a file handle to the specified volume and parses its volume boot record.
func GetVolumeHandler(volumeLetter string, handler handler) (volume VolumeHandler, err error) {
	const volumeBootRecordSize = 512
	volume.VolumeLetter = volumeLetter
	volume.Handle, err = handler.GetHandle(volumeLetter)
	if err != nil {
		err = fmt.Errorf("GetVolumeHandler() failed to get handle to volume %s: %w", volumeLetter, err)
		return
	}

	// Parse the VBR to get details we need about the volume.
	volumeBootRecord := make([]byte, volumeBootRecordSize)
	_, err = volume.Handle.Read(volumeBootRecord)
	if err != nil {
		err = fmt.Errorf("GetVolumeHandler() failed to read the volume boot record on volume %v: %w", volumeLetter, err)
		return
	}
	volume.Vbr, err = vbr.RawVolumeBootRecord(volumeBootRecord).Parse()
	if err != nil {
		err = fmt.Errorf("GetVolumeHandler() failed to parse vbr from volume letter %s: %w", volumeLetter, err)
		return
	}
	log.Debugf("Successfully got a file handle to volume %v and read its volume boot record.", volumeLetter)
	return
}

func isLetter(s string) (result bool, err error) {
	// Sanity checking
	if s == "" {
		err = errors.New("isLetter() received a null string")
		return
	} else if len(s) > 1 {
		err = fmt.Errorf("isLetter() received the string %s which is too many letters, function expected a single letter", s)
		return
	}

	for _, r := range s {
		if unicode.IsLetter(r) {
			result = true
			return
		}
	}

	result = false
	return
}

func identifyVolumesOfInterest(exportList *ListOfFilesToExport) (volumesOfInterest []string, err error) {
	volumesOfInterest = make([]string, 0)
	re := regexp.MustCompile(`[^:]+`)
	for _, fileToExport := range *exportList {
		volume := re.FindString(strings.ToLower(fileToExport.FullPath))
		var result bool
		result, err = isLetter(volume)
		if err != nil {
			err = fmt.Errorf("isLetter() returned an error: %w", err)
			volumesOfInterest = nil
			return
		} else if result == false {
			err = fmt.Errorf("isLetter() indicated that the full path string %s does not start with a letter", fileToExport.FullPath)
			volumesOfInterest = nil
			return
		}

		isTracked := false
		for _, trackedVolumes := range volumesOfInterest {
			if trackedVolumes == volume {
				isTracked = true
				break
			}
		}

		if isTracked == true {
			continue
		} else {
			volumesOfInterest = append(volumesOfInterest, volume)
		}
	}

	return
}
