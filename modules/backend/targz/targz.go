package targz

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path"
	"regexp"

	"github.com/juju/ratelimit"
	"github.com/klauspost/pgzip"
)

const regexToIgnoreErr = "^tar:.*(Removing leading|socket ignored|file changed as we read it|Удаляется начальный|сокет проигнорирован|файл изменился во время чтения)"

type Error struct {
	Err    error
	Stderr string
}

func (e Error) Error() string {
	return e.Err.Error()
}

type limitedWriteCloser struct {
	w io.Writer
	c io.Closer
}

func (lwc *limitedWriteCloser) Write(p []byte) (int, error) {
	return lwc.w.Write(p)
}

func (lwc *limitedWriteCloser) Close() error {
	return lwc.c.Close()
}

func GetFileWriter(filePath string, gZip bool, rateLim int64) (io.WriteCloser, error) {
	var wc io.WriteCloser

	file, err := os.Create(filePath)
	if err != nil {
		return nil, err
	}

	lwc := &limitedWriteCloser{
		c: file,
	}
	if rateLim != 0 {
		bucket := ratelimit.NewBucketWithRate(float64(rateLim), rateLim*2)
		lwc.w = ratelimit.Writer(file, bucket)
	} else {
		lwc.w = file
	}

	if gZip {
		wc, err = pgzip.NewWriterLevel(lwc, pgzip.BestCompression)
	} else {
		wc = lwc
	}

	return wc, err
}

func GZip(src, dst string, rateLim int64) error {
	fileWriter, err := GetFileWriter(dst, true, rateLim)
	if err != nil {
		return err
	}
	defer func() { _ = fileWriter.Close() }()

	file, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	_, err = io.Copy(fileWriter, file)
	return err
}

func Tar(src, dst string, incremental, gzip, saveAbsPath bool, rateLim int64, excludes []string) error {

	tarWriter, err := GetFileWriter(dst, gzip, rateLim)
	if err != nil {
		return err
	}
	defer func() { _ = tarWriter.Close() }()

	var stderr bytes.Buffer
	var args []string

	args = append(args, "--format=pax")

	if incremental {
		args = append(args, "--listed-incremental="+dst+".inc")
	}
	for _, ex := range excludes {
		args = append(args, "--exclude="+ex)

	}
	args = append(args, "--ignore-failed-read")
	args = append(args, "--create")
	args = append(args, "--file=-")
	if saveAbsPath {
		args = append(args, src)
	} else {
		args = append(args, "--directory="+path.Dir(src))
		args = append(args, path.Base(src))
	}

	cmd := exec.Command("tar", args...)
	cmd.Stdout = tarWriter
	cmd.Stderr = &stderr

	if err = cmd.Run(); err != nil {
		if cmd.ProcessState.ExitCode() == 2 || checkIsRealError(stderr.String()) {
			return Error{
				Err:    err,
				Stderr: stderr.String(),
			}
		}
	}

	return nil
}

func checkIsRealError(stderr string) bool {
	realErr := false
	reTar := regexp.MustCompile("^tar:.*\n")
	reErr := regexp.MustCompile(regexToIgnoreErr)
	strTupl := reTar.FindAllString(stderr, -1)
	for _, s := range strTupl {
		if match := reErr.MatchString(s); !match {
			realErr = true
			break
		}
	}

	return realErr
}
