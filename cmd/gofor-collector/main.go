// Copyright (c) 2020 Alec Randazzo

package main

import (
	"archive/zip"
	"fmt"
	collector "github.com/Go-Forensics/Windows-Collector"
	"github.com/jessevdk/go-flags"
	log "github.com/sirupsen/logrus"
	"os"
	"strings"
)

type options struct {
	Debug string `short:"d" long:"debug" default:"" description:"Log debug information to output file."`
	//SendTo             string   `short:"s" long:"sendto" required:"true" description:"Where to send collected files to." choice:"zip"`
	ZipName            string `short:"z" long:"zipname" description:"Output file name for the zip." required:"true"`
	DataTypesToCollect string `short:"g" long:"gather" default:"a" description:"Types of data to collect. Concatenate the abbreviation characters together for what you want. The order doesn't matter. Valid values are 'a' for all, 'm' for $MFT, 'r' for system registries, 'u' for user registries, 'e' for event logs, 'w' for web history. Examples: '/g mrue', '/g a'"`
}

func init() {
	// Log configuration
	log.SetFormatter(&log.JSONFormatter{})
	// runtime.GOMAXPROCS(1)
}

func main() {
	opts := new(options)
	parsedOpts := flags.NewParser(opts, flags.Default)
	_, err := parsedOpts.Parse()
	if err != nil {
		os.Exit(-1)
	}

	log.SetFormatter(&log.JSONFormatter{})
	if opts.Debug == "" {
		log.SetOutput(os.Stdout)
		log.SetLevel(log.ErrorLevel)
	} else {
		debugLog, _ := os.Create(opts.Debug)
		defer debugLog.Close()
		log.SetOutput(debugLog)
		log.SetLevel(log.DebugLevel)
	}

	var exportList collector.ListOfFilesToExport
	if strings.Contains(opts.DataTypesToCollect, "a") {
		exportList = collector.ListOfFilesToExport{
			{
				FullPath:        `%SYSTEMDRIVE%:\$MFT`,
				IsFullPathRegex: false,
				FileName:        `$MFT`,
				IsFileNameRegex: false,
			},
			{
				FullPath:        `%SYSTEMDRIVE%:\Windows\System32\config\SYSTEM`,
				IsFullPathRegex: false,
				FileName:        `SYSTEM`,
				IsFileNameRegex: false,
			},
			{
				FullPath:        `%SYSTEMDRIVE%:\Windows\System32\config\SOFTWARE`,
				IsFullPathRegex: false,
				FileName:        `SOFTWARE`,
				IsFileNameRegex: false,
			},
			{
				FullPath:        `%SYSTEMDRIVE%:\\Windows\\System32\\winevt\\Logs\\.*\.evtx$`,
				IsFullPathRegex: true,
				FileName:        `.*\.evtx$`,
				IsFileNameRegex: true,
			},
			{
				FullPath:        `%SYSTEMDRIVE%:\\users\\([^\\]+)\\ntuser.dat`,
				IsFullPathRegex: true,
				FileName:        `ntuser.dat`,
				IsFileNameRegex: false,
			},
			{
				FullPath:        `%SYSTEMDRIVE%:\\Users\\([^\\]+)\\AppData\\Local\\Microsoft\\Windows\\usrclass.dat`,
				IsFullPathRegex: true,
				FileName:        `usrclass.dat`,
				IsFileNameRegex: false,
			},
			{
				FullPath:        `%SYSTEMDRIVE%:\\Users\\([^\\]+)\\AppData\\Local\\Microsoft\\Windows\\WebCache\\WebCacheV01.dat`,
				IsFullPathRegex: true,
				FileName:        `WebCacheV01.dat`,
				IsFileNameRegex: false,
			},
		}
	} else {
		if strings.Contains(opts.DataTypesToCollect, "m") {
			exportList = append(exportList, collector.FileToExport{
				FullPath:        `%SYSTEMDRIVE%:\$MFT`,
				IsFullPathRegex: false,
				FileName:        `$MFT`,
				IsFileNameRegex: false,
			})
		}
		if strings.Contains(opts.DataTypesToCollect, "r") {
			exportList = append(exportList, collector.FileToExport{
				FullPath:        `%SYSTEMDRIVE%:\Windows\System32\config\SYSTEM`,
				IsFullPathRegex: false,
				FileName:        `SYSTEM`,
				IsFileNameRegex: false,
			})
			exportList = append(exportList, collector.FileToExport{
				FullPath:        `%SYSTEMDRIVE%:\Windows\System32\config\SOFTWARE`,
				IsFullPathRegex: false,
				FileName:        `SOFTWARE`,
				IsFileNameRegex: false,
			})
		}
		if strings.Contains(opts.DataTypesToCollect, "u") {
			exportList = append(exportList, collector.FileToExport{
				FullPath:        `%SYSTEMDRIVE%:\\users\\([^\\]+)\\ntuser.dat`,
				IsFullPathRegex: true,
				FileName:        `ntuser.dat`,
				IsFileNameRegex: false,
			})
			exportList = append(exportList, collector.FileToExport{
				FullPath:        `%SYSTEMDRIVE%:\\Users\\([^\\]+)\\AppData\\Local\\Microsoft\\Windows\\usrclass.dat`,
				IsFullPathRegex: true,
				FileName:        `usrclass.dat`,
				IsFileNameRegex: false,
			})
		}
		if strings.Contains(opts.DataTypesToCollect, "e") {
			exportList = append(exportList, collector.FileToExport{
				FullPath:        `%SYSTEMDRIVE%:\\Windows\\System32\\winevt\\Logs\\.*\\.evtx$`,
				IsFullPathRegex: true,
				FileName:        `.*\\.evtx$`,
				IsFileNameRegex: true,
			})
		}
		if strings.Contains(opts.DataTypesToCollect, "w") {
			exportList = append(exportList, collector.FileToExport{
				FullPath:        `%SYSTEMDRIVE%:\\Users\\([^\\]+)\\AppData\\Local\\Microsoft\\Windows\\WebCache\\WebCacheV01.dat`,
				IsFullPathRegex: true,
				FileName:        `WebCacheV01.dat`,
				IsFileNameRegex: false,
			})
		}
	}

	fileHandle, err := os.Create(opts.ZipName)
	if err != nil {
		err = fmt.Errorf("failed to create zip file %s", opts.ZipName)
	}
	zipWriter := zip.NewWriter(fileHandle)
	resultWriter := collector.ZipResultWriter{
		ZipWriter:  zipWriter,
		FileHandle: fileHandle,
	}
	var volume collector.VolumeHandler
	err = collector.Collect(volume, exportList, &resultWriter)
	if err != nil {
		log.Panic(err)
	}
}
