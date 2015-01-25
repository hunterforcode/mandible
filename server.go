package main

import (
	"errors"
	"fmt"
	"github.com/gophergala/ImgurGo/imageprocessor"
	"github.com/gophergala/ImgurGo/imagestore"
	"github.com/gophergala/ImgurGo/uploadedfile"
	"io"
	"io/ioutil"
	"net/http"
	"os"
)

type Server struct {
	Config     *Configuration
	HTTPClient *http.Client
	imageStore imagestore.ImageStore
}

func CreateServer(c *Configuration) *Server {
	factory := Factory{c}
	httpclient := &http.Client{}
	store := factory.NewS3()
	return &Server{c, httpclient, store}
}

func (s *Server) _uploadFile(uploadFile io.ReadCloser, w http.ResponseWriter) {
	defer uploadFile.Close()

	tmpFile, err := ioutil.TempFile(os.TempDir(), "image")
	if err != nil {
		fmt.Println(err)
		ErrorResponse(w, "Unable to write to /tmp", http.StatusInternalServerError)
		return
	}

	defer tmpFile.Close()

	_, err = io.Copy(tmpFile, uploadFile)

	if err != nil {
		fmt.Println(err)
		ErrorResponse(w, "Unable to copy image to disk!", http.StatusInternalServerError)
		return
	}

	upload := uploadedfile.NewUploadedFile("testfile.jpg", tmpFile.Name(), "image/jpeg")
	processor, err := imageprocessor.Factory(s.Config.MaxFileSize, upload)
	if err != nil {
		fmt.Println(err)

		ErrorResponse(w, "Unable to process image!", http.StatusInternalServerError)
		return
	}

	err = processor.Run(upload)
	if err != nil {
		fmt.Println(err)
		ErrorResponse(w, "Unable to process image!", http.StatusInternalServerError)
		return
	}

	upload.SetHash(<-hashGetter)
	factory := Factory{s.Config}
	obj := factory.NewStoreObject(upload.GetHash(), upload.GetMime(), "original")
	err = s.imageStore.Save(upload.GetPath(), obj)

	if err != nil {
		ErrorResponse(w, "Unable to save image!", http.StatusInternalServerError)
		return
	}

	resp := make(map[string]interface{})

	// TODO: Build JSON respons

	Response(w, resp)
}

func (s *Server) initServer() {
	fileHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		uploadFile, _, err := r.FormFile("image")

		if err != nil {
			fmt.Println(err)
			ErrorResponse(w, "Error processing file!", http.StatusInternalServerError)
			return
		}

		s._uploadFile(uploadFile, w)
	}

	urlHandler := func(w http.ResponseWriter, r *http.Request) {
		uploadFile, err := s.download(r.FormValue("image"))

		if err != nil {
			ErrorResponse(w, "Error dowloading URL!", http.StatusInternalServerError)
			return
		}

		s._uploadFile(uploadFile, w)
	}

	http.HandleFunc("/file", fileHandler)
	http.HandleFunc("/url", urlHandler)

	http.ListenAndServe(fmt.Sprintf(":%d", s.Config.Port), nil)
}

func (s *Server) download(url string) (io.ReadCloser, error) {
	req, err := http.NewRequest("GET", url, nil)

	if err != nil {
		return nil, err
	}

	req.Header.Add("User-Agent", s.Config.UserAgent)

	resp, err := s.HTTPClient.Do(req)

	if err != nil {
		// "HTTP protocol error" - maybe the server sent an invalid response or timed out
		return nil, err
	}

	if 200 != resp.StatusCode {
		return nil, errors.New("Non-200 status code received")
	}

	contentLength := resp.ContentLength

	if contentLength == 0 {
		return nil, errors.New("Empty file received")
	}

	return resp.Body, nil
}
