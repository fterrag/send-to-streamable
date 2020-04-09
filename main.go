package main

//go:generate goversioninfo -icon=streamable.ico -manifest=sts.exe.manifest

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
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

	"golang.org/x/crypto/ssh/terminal"
)

var httpClient = &http.Client{}

type conf struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func main() {
	flagSetConf := flag.Bool("config", false, "set configuration")
	flag.Parse()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("Error getting user home directory: %s", err)
		os.Exit(1)
	}

	confPath := fmt.Sprintf("%s\\.stsconf", homeDir)

	if *flagSetConf {
		fmt.Println("Enter your Streamable credentials")

		for {
			email := userInput("Email", false)
			password := userInput("Password", true)

			err = checkAuth(httpClient, email, password)
			if err != nil {
				fmt.Printf("Authentication failed. Try again or press Control-C to exit.\n\n")
				continue
			}

			c := conf{
				Email:    email,
				Password: password,
			}

			b, err := json.Marshal(c)
			if err != nil {
				fmt.Println("Error marshaling configuration JSON")
				os.Exit(1)
			}

			err = ioutil.WriteFile(confPath, b, 0600)
			if err != nil {
				fmt.Printf("Error writing configuration file file: %s", err)
				os.Exit(1)
			}

			fmt.Println("Configuration successfully saved. Press Enter to exit.")

			reader := bufio.NewReader(os.Stdin)
			reader.ReadString('\n')

			os.Exit(0)
		}
	}

	b, err := ioutil.ReadFile(confPath)
	if err != nil {
		fmt.Println("Error reading configuration file")
		os.Exit(1)
	}

	conf := &conf{}
	err = json.Unmarshal(b, conf)
	if err != nil {
		fmt.Printf("Error unmarshaling configuration JSON")
		os.Exit(1)
	}

	if len(os.Args) < 1 {
		os.Exit(0)
	}

	filename := os.Args[1]

	title := userInput("Video Title", false)

	fmt.Printf("Uploading video: %s\n\n", filename)

	u, err := uploadVideo(httpClient, conf.Email, conf.Password, filename, title)
	if err != nil {
		fmt.Printf("Error uploading video: %s", err)
		os.Exit(1)
	}

	fmt.Printf("Video URL: %s\n", u)
	fmt.Printf("\nPress Enter to exit\n")

	reader := bufio.NewReader(os.Stdin)
	reader.ReadString('\n')
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
