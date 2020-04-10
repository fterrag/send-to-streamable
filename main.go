package main

//go:generate goversioninfo -icon=streamable.ico -manifest=sts.exe.manifest

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"fyne.io/fyne/app"
	"fyne.io/fyne/dialog"
	"fyne.io/fyne/widget"
	"golang.org/x/crypto/ssh/terminal"
)

var httpClient = &http.Client{}

type conf struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func main() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("Error getting user home directory: %s", err)
		os.Exit(1)
	}

	confPath := fmt.Sprintf("%s\\.stsconf", homeDir)
	conf := &conf{}

	if _, err := os.Stat(confPath); err == nil {
		b, _ := ioutil.ReadFile(confPath)
		json.Unmarshal(b, conf)
	}

	app := app.New()

	w := app.NewWindow("STS")
	w.SetFixedSize(true)

	if len(os.Args) > 1 {
		uploaded := false

		filename := widget.NewEntry()
		filename.SetText(os.Args[1])
		filename.Disable()
		title := widget.NewEntry()
		videoURL := widget.NewEntry()
		videoURL.Disable()

		form := &widget.Form{
			OnSubmit: func() {
				if len(conf.Email) == 0 || len(conf.Password) == 0 {
					dialog.NewInformation("", "Missing Streamable credentials.", w).Show()
					return
				}

				if uploaded {
					dialog.NewInformation("", "Video has already been uploaded.", w).Show()
					return
				}

				url, err := uploadVideo(httpClient, conf.Email, conf.Password, filename.Text, title.Text)
				if err != nil {
					dialog.NewInformation("", "Error uploading video", w).Show()
					return
				}

				uploaded = true
				videoURL.SetText(url)
				videoURL.Enable()
			},
		}

		form.Append("Filename", filename)
		form.Append("Title", title)
		form.Append("Video URL", videoURL)

		w.Canvas().Focus(title)

		w.SetContent(widget.NewVBox(
			form,
		))

		w.ShowAndRun()
		return
	}

	email := widget.NewEntry()
	email.SetPlaceHolder("you@somewhere.com")
	password := widget.NewPasswordEntry()
	password.SetPlaceHolder("Password")

	form := &widget.Form{
		OnSubmit: func() {
			if len(email.Text) == 0 || len(password.Text) == 0 {
				return
			}

			err = checkAuth(httpClient, email.Text, password.Text)
			if err != nil {
				dialog.NewInformation("", "Invalid credentials.", w).Show()
				return
			}

			conf.Email = email.Text
			conf.Password = password.Text

			b, err := json.Marshal(conf)
			if err != nil {
				dialog.NewInformation("", "Error marshaling JSON", w).Show()
				return
			}

			err = ioutil.WriteFile(confPath, b, 0600)
			if err != nil {
				dialog.NewInformation("", "Error writing configuration file", w).Show()
				return
			}

			dialog.NewInformation("", "STS is now configured.", w).Show()
		},
	}

	form.Append("Email", email)
	form.Append("Password", password)

	w.Canvas().Focus(email)

	w.SetContent(widget.NewVBox(
		widget.NewLabel("Enter your Streamable email and password and click Submit."),
		form,
	))

	w.ShowAndRun()
}

func userInput(label string, password bool) string {
	var out string

	fmt.Printf("%s: ", label)

	if password {
		password, _ := terminal.ReadPassword(int(syscall.Stdin))
		fmt.Println("")
		out = string(password)
	} else {
		reader := bufio.NewReader(os.Stdin)
		out, _ = reader.ReadString('\n')
	}

	return strings.TrimSpace(out)
}

func checkAuth(httpClient *http.Client, username, password string) error {
	req, err := http.NewRequest("POST", "https://api.streamable.com/upload", nil)
	if err != nil {
		return err
	}

	req.Header.Add("User-Agent", "send-to-streamable/1.0")
	req.SetBasicAuth(username, password)

	res, err := httpClient.Do(req)
	if err != nil {
		return err
	}

	if res.StatusCode == 401 {
		return errors.New("Invalid credentials")
	}

	return nil
}

func uploadVideo(httpClient *http.Client, username, password, filename, title string) (string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", filepath.Base(file.Name()))

	if err != nil {
		return "", err
	}

	io.Copy(part, file)
	writer.Close()

	req, err := http.NewRequest("POST", fmt.Sprintf("https://api.streamable.com/upload?title=%s", url.QueryEscape(title)), body)
	if err != nil {
		return "", err
	}

	req.Header.Add("User-Agent", "send-to-stremable/1.0")
	req.SetBasicAuth(username, password)
	req.Header.Add("Content-Type", writer.FormDataContentType())

	res, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}

	if res.StatusCode != 200 {
		return "", errors.New("Streamable responded with an error")
	}

	b, _ := ioutil.ReadAll(res.Body)
	res.Body.Close()

	type uploadRes struct {
		Shortcode string `json:"shortcode"`
		Status    int    `json:"status"`
	}

	uRes := &uploadRes{}

	json.Unmarshal(b, &uRes)

	return fmt.Sprintf("https://streamable.com/%s", uRes.Shortcode), nil
}
