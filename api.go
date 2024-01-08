package gogpt

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"
	"time"

	"github.com/goccy/go-json"
)

const apiURLv1 = "https://api.openai.com/v1"

func newTransport() *http.Client {
	return &http.Client{
		Timeout: time.Minute,
	}
}

// Client is OpenAI GPT-3 API client.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	authToken  string
	idOrg      string
}

// NewClient creates new OpenAI API client.
func NewClient(authToken string) *Client {
	return &Client{
		BaseURL:    apiURLv1,
		HTTPClient: newTransport(),
		authToken:  authToken,
		idOrg:      "",
	}
}

func NewClientWithHTTPClient(authToken string, httpClient *http.Client) *Client {
	return &Client{
		BaseURL:    apiURLv1,
		HTTPClient: httpClient,
		authToken:  authToken,
		idOrg:      "",
	}
}

// NewOrgClient creates new OpenAI API client for specified Organization ID.
func NewOrgClient(authToken, org string) *Client {
	return &Client{
		BaseURL:    apiURLv1,
		HTTPClient: newTransport(),
		authToken:  authToken,
		idOrg:      org,
	}
}

func (c *Client) sendRequest(req *http.Request, v interface{}) error {
	req.Header.Set("Accept", "application/json; charset=utf-8")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.authToken))

	// Check whether Content-Type is already set, Upload Files API requires
	// Content-Type == multipart/form-data
	contentType := req.Header.Get("Content-Type")
	if contentType == "" {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}

	if len(c.idOrg) > 0 {
		req.Header.Set("OpenAI-Organization", c.idOrg)
	}

	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}

	defer res.Body.Close()

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusBadRequest {
		var errRes ErrorResponse
		err = json.NewDecoder(res.Body).Decode(&errRes)
		if err != nil || errRes.Error == nil {
			return fmt.Errorf("error, status code: %d", res.StatusCode)
		}
		return fmt.Errorf("error, status code: %d, message: %s", res.StatusCode, errRes.Error.Message)
	}

	if v != nil {
		if err = json.NewDecoder(res.Body).Decode(&v); err != nil {
			return err
		}
	}

	return nil
}

var (
	dataPrefix   = []byte("data: ")
	doneSequence = []byte("[DONE]")
)

func (c *Client) sendStreamRequest(req *http.Request, onData func(CompletionResponse)) (output CompletionResponse, err error) {
	req.Header.Set("Accept", "application/json; charset=utf-8")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.authToken))

	// Check whether Content-Type is already set, Upload Files API requires
	// Content-Type == multipart/form-data
	contentType := req.Header.Get("Content-Type")
	if contentType == "" {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}

	if len(c.idOrg) > 0 {
		req.Header.Set("OpenAI-Organization", c.idOrg)
	}

	var res *http.Response
	res, err = c.HTTPClient.Do(req)
	if err != nil {
		return
	}

	defer res.Body.Close()

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusBadRequest {
		var errRes ErrorResponse
		err = json.NewDecoder(res.Body).Decode(&errRes)
		if err != nil || errRes.Error == nil {
			err = fmt.Errorf("error, status code: %d", res.StatusCode)
			return
		}
		err = fmt.Errorf("error, status code: %d, message: %s", res.StatusCode, errRes.Error.Message)
		return
	}

	reader := bufio.NewReader(res.Body)
	output = CompletionResponse{}

	var prevText string
	var line []byte

	for {
		line, err = reader.ReadBytes('\n')
		if err != nil {
			return
		}
		// make sure there isn't any extra whitespace before or after
		line = bytes.TrimSpace(line)
		// the completion API only returns data events
		if !bytes.HasPrefix(line, dataPrefix) {
			continue
		}
		line = bytes.TrimSpace(bytes.TrimPrefix(line, dataPrefix))

		// the stream is completed when terminated by [DONE]
		if bytes.HasPrefix(line, doneSequence) {
			break
		}

		if len(output.Choices) > 0 {
			prevText = output.Choices[0].Text
		}

		if err = json.Unmarshal(line, &output); err != nil {
			err = fmt.Errorf("invalid json stream data: %v", err)
			return
		}

		if len(output.Choices) > 0 {
			output.Choices[0].Text = prevText + output.Choices[0].Text
		}

		onData(output)
	}

	return
}

func (c *Client) fullURL(suffix string) string {
	return fmt.Sprintf("%s%s", c.BaseURL, suffix)
}
