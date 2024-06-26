package self_update

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/nixys/nxs-backup/misc"
)

type Opts struct {
	Version string
	Done    chan error
}

type selfUpdate struct {
	version string
	done    chan error
}

func Init(o Opts) *selfUpdate {
	return &selfUpdate{
		version: o.Version,
		done:    o.Done,
	}
}

func (su *selfUpdate) Run() {
	var tmpBinFile *os.File

	newVer, url, err := misc.CheckNewVersionAvailable(su.version)
	if err != nil {
		printSelfUpErr(su.done, err)
		return
	}

	if newVer == "" {
		fmt.Println("No new versions.")
		su.done <- nil
		return
	}
	fmt.Printf("The new version is: %s\n", newVer)
	exePath, err := os.Executable()
	if err != nil {
		printSelfUpErr(su.done, err)
		return
	}
	tarPath := exePath + ".tgz"
	newExePath := exePath + "-new"

	tarFile, err := os.Create(tarPath)
	if err != nil {
		printSelfUpErr(su.done, err)
		return
	}
	defer func() { _ = os.Remove(tarFile.Name()) }()

	resp, err := http.Get(url)
	if err != nil {
		printSelfUpErr(su.done, err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	_, err = io.Copy(tarFile, resp.Body)
	if err != nil {
		printSelfUpErr(su.done, err)
		return
	}
	defer func() { _ = tarFile.Close() }()

	_, err = tarFile.Seek(0, 0)
	if err != nil {
		printSelfUpErr(su.done, err)
		return
	}

	gr, err := gzip.NewReader(tarFile)
	if err != nil {
		printSelfUpErr(su.done, err)
		return
	}
	defer func() { _ = gr.Close() }()

	tr := tar.NewReader(gr)

	tmpBinFile, err = os.OpenFile(newExePath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		printSelfUpErr(su.done, err)
		return
	}
	defer func() { _ = os.Remove(tmpBinFile.Name()) }()

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			printSelfUpErr(su.done, err)
			return
		}

		if header.Name == "./nxs-backup" {
			if _, err = io.Copy(tmpBinFile, tr); err != nil {
				printSelfUpErr(su.done, err)
				return
			}
			break
		}
	}

	err = tmpBinFile.Close()
	if err != nil {
		printSelfUpErr(su.done, err)
		return
	}

	err = os.Rename(tmpBinFile.Name(), exePath)
	if err != nil {
		printSelfUpErr(su.done, err)
		return
	}

	fmt.Println("Update completed.")
	su.done <- nil
}

func printSelfUpErr(dc chan error, err error) {
	_, _ = fmt.Fprintf(os.Stderr, "Selfupdate failed: %v\n", err)
	dc <- err
}
