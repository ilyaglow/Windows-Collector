// Copyright (c) 2020 Alec Randazzo

package windowscollector

import (
	"fmt"
	"io"
	"sync"

	mft "github.com/Go-Forensics/MFT-Parser"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/windows/registry"
)

func resolveEnv(exportList ListOfFilesToExport) error {
	var err error
	for i := range exportList {
		exportList[i].FullPath, err = registry.ExpandString(exportList[i].FullPath)
		if err != nil {
			return fmt.Errorf("resolving environment variables from %s: %w", exportList[i].FullPath, err)
		}
	}
	return nil
}

// Collect will find and collect target files into a format depending on the resultWriter type
func Collect(injectedHandlerDependency handler, exportList ListOfFilesToExport, resultWriter resultWriter) (err error) {
	// volumeHandler as an arg is a dependency injection
	err = resolveEnv(exportList)
	if err != nil {
		return
	}

	log.Debugf("Attempting to acquire the following files %+v", exportList)
	volumesOfInterest, err := identifyVolumesOfInterest(&exportList)
	if err != nil {
		err = fmt.Errorf("identifyVolumesOfInterest() returned an error: %w", err)
		return
	}

	searchTerms, err := setupSearchTerms(exportList)
	if err != nil {
		err = fmt.Errorf("setupSearchTerms() returned the following error: %w", err)
		return
	}

	for _, volumeLetter := range volumesOfInterest {
		var volumeHandler VolumeHandler
		volumeHandler, err = GetVolumeHandler(volumeLetter, injectedHandlerDependency)
		if err != nil {
			err = fmt.Errorf("GetVolumeHandler() failed to get a handle to the volume %s: %w", volumeLetter, err)
			return
		}

		err = getFiles(&volumeHandler, resultWriter, searchTerms)
		if err != nil {
			err = fmt.Errorf("getFiles() failed to get files: %w", err)
			return
		}
	}
	return
}

func getFiles(volumeHandler *VolumeHandler, resultWriter resultWriter, listOfSearchKeywords listOfSearchTerms) (err error) {
	// Init a few things
	fileReaders := make(chan fileReader, 100)
	waitForFileCopying := sync.WaitGroup{}
	waitForFileCopying.Add(1)
	go resultWriter.ResultWriter(fileReaders, &waitForFileCopying)

	// parse the mft's mft record to get its dataruns
	mftRecord0, err := parseMFTRecord0(volumeHandler)
	if err != nil {
		err = fmt.Errorf("parseMFTRecord0() failed to parse mft record 0 from the volume %s: %w", volumeHandler.VolumeLetter, err)
		return
	}
	log.Debugf("Parsed the MFT's MFT record and got the following: %+v", mftRecord0)

	// Go back to the beginning of the mft record
	_, _ = volumeHandler.Handle.Seek(volumeHandler.Vbr.MftByteOffset, 0)
	log.Debugf("Seeked back to the beginning offset to the MFT at offset %d", volumeHandler.Vbr.MftByteOffset)

	// Open a raw reader on the MFT
	foundFile := foundFile{
		dataRuns: mftRecord0.DataAttribute.NonResidentDataAttribute.DataRuns,
		fullPath: "$mft",
	}
	mftReader := rawFileReader(volumeHandler, foundFile)
	log.Debug("Obtained a raw io.Reader to the MFT's dataruns.")

	// Do we need to stream a copy of the mft while we read it?
	areWeCopyingTheMFT := false
	directoryTree := mft.DirectoryTree{}
	possibleMatches := possibleMatches{}

	for index, value := range listOfSearchKeywords {
		if value.fileNameString == "$mft" {
			areWeCopyingTheMFT = true

			// delete this from our search list
			listOfSearchKeywords[index] = listOfSearchKeywords[len(listOfSearchKeywords)-1]
			listOfSearchKeywords = listOfSearchKeywords[:len(listOfSearchKeywords)-1]

			break
		}
	}

	if areWeCopyingTheMFT == true {
		log.Debug("We are configured to grab a copy of the MFT, so we'll set up a io.TeeReader with an io.Pipe so we can copy the mft as we read it. We do this so we only have to read the MFT's data runs once and only once.")
		pipeReader, pipeWriter := io.Pipe()
		teeReader := io.TeeReader(mftReader, pipeWriter)
		fileReader := fileReader{
			fullPath: fmt.Sprintf("%s__$mft", volumeHandler.VolumeLetter),
			reader:   pipeReader,
		}
		fileReaders <- fileReader
		volumeHandler.mftReader = teeReader
		possibleMatches, directoryTree, err = findPossibleMatches(volumeHandler, listOfSearchKeywords)
		if err != nil {
			err = fmt.Errorf("findPossibleMatches() failed: %w", err)
			return
		}
		err = pipeWriter.Close()
		if err != nil {
			err = fmt.Errorf("failed to close writer pipe: %w", err)
			return
		}
	} else {
		volumeHandler.mftReader = mftReader
		possibleMatches, directoryTree, err = findPossibleMatches(volumeHandler, listOfSearchKeywords)
		if err != nil {
			err = fmt.Errorf("findPossibleMatches() failed: %w", err)
			return
		}
	}

	foundFiles := confirmFoundFiles(listOfSearchKeywords, possibleMatches, directoryTree)
	if err != nil {
		err = fmt.Errorf("confirmFoundFiles() failed with error: %w", err)
		return
	}

	for _, file := range foundFiles {
		// try to get an io.reader via api first
		reader, err := apiFileReader(file)
		if err != nil {
			log.Debugf("Got a raw io.Reader for '%s' with data runs: %+v", file.fullPath, file.dataRuns)
			// failed to get an API handle, trying to get an io.reader via raw method
			reader = rawFileReader(volumeHandler, file)
		} else {
			log.Debugf("Got an API io.Reader for '%s'.", file.fullPath)
		}
		fileReader := fileReader{
			fullPath: file.fullPath,
			reader:   reader,
		}
		fileReaders <- fileReader
	}
	close(fileReaders)
	err = nil
	waitForFileCopying.Wait()
	return
}
