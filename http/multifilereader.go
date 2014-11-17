package http

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"

	cmds "github.com/jbenet/go-ipfs/commands"
)

type MultiFileReader struct {
	io.Reader

	files       cmds.File
	currentFile io.Reader
	buf         bytes.Buffer
	mpWriter    *multipart.Writer
	closed      bool

	// if true, the data will be type 'multipart/form-data'
	// if false, the data will be type 'multipart/mixed'
	form bool
}

func NewMultiFileReader(file cmds.File, form bool) *MultiFileReader {
	mfr := &MultiFileReader{
		files: file,
		form:  form,
	}
	mfr.mpWriter = multipart.NewWriter(&mfr.buf)

	return mfr
}

func (mfr *MultiFileReader) Read(buf []byte) (written int, err error) {
	// if we are closed, end reading
	if mfr.closed && mfr.buf.Len() == 0 {
		return 0, io.EOF
	}

	// if the current file isn't set, advance to the next file
	if mfr.currentFile == nil {
		file, err := mfr.files.NextFile()
		if err == io.EOF || (err == nil && file == nil) {
			mfr.mpWriter.Close()
			mfr.closed = true
		} else if err != nil {
			return 0, err
		}

		// handle starting a new file part
		if !mfr.closed {
			if file.IsDirectory() {
				// if file is a directory, create a multifilereader from it
				// (using 'multipart/mixed')
				mfr.currentFile = NewMultiFileReader(file, false)
			} else {
				// otherwise, use the file as a reader to read its contents
				mfr.currentFile = file
			}

			// write the boundary and headers
			header := make(textproto.MIMEHeader)
			if mfr.form {
				contentDisposition := fmt.Sprintf("form-data; name=\"file\"; filename=\"%s\"", file.FileName())
				header.Set("Content-Disposition", contentDisposition)
			} else {
				header.Set("Content-Disposition", fmt.Sprintf("file; filename=\"%s\"", file.FileName()))
			}

			if file.IsDirectory() {
				boundary := mfr.currentFile.(*MultiFileReader).Boundary()
				header.Set("Content-Type", fmt.Sprintf("multipart/mixed; boundary=%s", boundary))
			} else {
				header.Set("Content-Type", "application/octet-stream")
			}

			_, err := mfr.mpWriter.CreatePart(header)
			if err != nil {
				return 0, err
			}
		}
	}

	var reader io.Reader

	if mfr.buf.Len() > 0 {
		// if the buffer has something in it, read from it
		reader = &mfr.buf

	} else if mfr.currentFile != nil {
		// otherwise, read from file data
		reader = mfr.currentFile
	}

	written, err = reader.Read(buf)
	if err == io.EOF && reader == mfr.currentFile {
		mfr.currentFile = nil
		return mfr.Read(buf)
	}
	return written, err
}

func (mfr *MultiFileReader) Boundary() string {
	return mfr.mpWriter.Boundary()
}
