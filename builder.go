package http_request

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type RequestBuilder struct {
	buildErr error

	header http.Header

	log     bool
	logFile string

	enableCompress  bool
	disableRedirect bool
	client          *http.Client
}

func New() *RequestBuilder {
	return &RequestBuilder{
		header: make(http.Header),
	}
}

func (c *RequestBuilder) Header(name string, value string) *RequestBuilder {
	c.header.Set(name, value)
	return c
}

func (c *RequestBuilder) WithProxy(proxyURL string) *RequestBuilder {
	if proxyURL == "" {
		return c
	}
	if c.buildErr != nil {
		return c
	}
	proxyUrl, err := url.Parse(proxyURL)
	if err != nil {
		c.buildErr = err
		return c
	}
	proxyTransport := &http.Transport{Proxy: http.ProxyURL(proxyUrl)}
	if c.client == nil {
		c.client = &http.Client{Transport: proxyTransport}
	} else {
		clone := *c.client
		clone.Transport = proxyTransport
		c.client = &clone
	}
	return c
}

func (c *RequestBuilder) WithClient(client *http.Client) *RequestBuilder {
	c.client = client
	return c
}

func (c *RequestBuilder) Compressed() *RequestBuilder {
	c.enableCompress = true
	return c
}

func (c *RequestBuilder) DisableRedirect() *RequestBuilder {
	c.disableRedirect = true
	return c
}

func unmarshalSafeNumber(body []byte, res interface{}) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	return dec.Decode(&res)
}

func (c *RequestBuilder) Log(v ...bool) *RequestBuilder {
	if len(v) > 0 {
		c.log = v[0]
	} else {
		c.log = true
	}
	return c
}

func (c *RequestBuilder) LogFile(logFile string) *RequestBuilder {
	c.logFile = logFile
	return c
}

func (c *RequestBuilder) request(ctx context.Context, url string, post bool, data interface{}, needData bool) (resp []byte, err error) {
	if c.buildErr != nil {
		return nil, c.buildErr
	}
	var needLog bool
	var logDataBytes []byte
	var logDataString string
	if c.log || c.logFile != "" {
		needLog = true
	}
	var bodyReader io.Reader
	var jsonContent bool
	var gzipped bool
	method := "GET"
	if post {
		jsonContent = true
		method = "POST"
		if data != nil {
			switch data := data.(type) {
			case []byte:
				bodyReader = bytes.NewReader(data)
				logDataBytes = data
			case json.RawMessage:
				bodyReader = bytes.NewReader(data)
				logDataBytes = data
			case string:
				bodyReader = strings.NewReader(data)
				logDataString = data
			default:
				var dataBytes []byte
				dataBytes, err = json.Marshal(data)
				if err != nil {
					return
				}
				bodyReader = bytes.NewReader(dataBytes)
				logDataBytes = dataBytes
			}
			if bodyReader != nil && c.enableCompress {
				gzipData, gzErr := gzipData(bodyReader)
				if gzErr != nil {
					return nil, fmt.Errorf("compress body err: %+v", gzErr)
				}
				bodyReader = bytes.NewReader(gzipData)
				gzipped = true
			}
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}
	// apply headers
	for header, values := range c.header {
		for _, value := range values {
			httpReq.Header.Add(header, value)
		}
	}
	if jsonContent {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	if needLog {
		var args []string
		args = append(args, "curl", "-v", "-X", method)
		for k, v := range httpReq.Header {
			for _, e := range v {
				args = append(args, "-H", fmt.Sprintf(`"%s: %s"`, k, e))
			}
		}
		if len(logDataBytes) > 0 {
			args = append(args, "--data-binary", quoteSh(string(logDataBytes)))
		} else if len(logDataString) > 0 {
			args = append(args, "--data-binary", quoteSh(logDataString))
		}
		args = append(args, quoteSh(url))

		cmdLog := strings.Join(args, " ")
		if c.logFile != "" {
			err := ioutil.WriteFile(c.logFile, []byte(cmdLog), 0755)
			if err != nil {
				return nil, fmt.Errorf("log err: %w", err)
			}
		}
		if c.log {
			fmt.Fprintf(os.Stderr, "HTTP DEBUG: %s\n", cmdLog)
		}
	}
	if gzipped {
		httpReq.Header.Set("Content-Encoding", "gzip")
	}
	client := http.DefaultClient
	if c.client != nil {
		client = c.client
	}
	if c.disableRedirect {
		cloneClient := *client
		cloneClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return ErrRedirect
		}
		client = &cloneClient
	}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	readData := needData
	var body []byte

	if readData {
		body, err = ioutil.ReadAll(httpResp.Body)
		if err != nil {
			return nil, err
		}
	} else {
		io.Copy(ioutil.Discard, httpResp.Body)
	}
	if httpResp.StatusCode >= 300 {
		return nil, fmt.Errorf("response err: %v %v %v", httpResp.StatusCode, httpResp.Status, string(body))
	}
	return body, nil
}

func quoteSh(s string) string {
	if !strings.Contains(s, "'") {
		return "'" + s + "'"
	}
	return strconv.Quote(s)
}

func gzipData(reader io.Reader) (compressedData []byte, err error) {
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)

	_, err = io.Copy(gz, reader)
	if err != nil {
		return
	}

	if err = gz.Flush(); err != nil {
		return
	}

	if err = gz.Close(); err != nil {
		return
	}

	compressedData = b.Bytes()

	return
}
