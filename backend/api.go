package backend

import (
	"app/generated"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"google.golang.org/protobuf/proto"
)

type Api struct {
	androidAPIVersion int64
	model             string
	make              string
	clientVersionCode int64
	userAgent         string
	language          string
	authData          string
	client            *http.Client
	authResponseCache map[string]string
	Email             string
}

func (a *Api) doProtobufPOST(endpoint string, requestData []byte) ([]byte, error) {
	bearerToken, err := a.BearerToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get bearer token: %w", err)
	}

	headers := map[string]string{
		"Accept-Encoding":          "gzip",
		"Accept-Language":          a.language,
		"Content-Type":             "application/x-protobuf",
		"User-Agent":               a.userAgent,
		"Authorization":            "Bearer " + bearerToken,
		"x-goog-ext-173412678-bin": "CgcIAhClARgC",
		"x-goog-ext-174067345-bin": "CgIIAg==",
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(requestData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var reader io.Reader = resp.Body
		if resp.Header.Get("Content-Encoding") == "gzip" {
			gz, gzErr := gzip.NewReader(resp.Body)
			if gzErr != nil {
				return nil, fmt.Errorf("request failed with status %d (gzip reader error: %v)", resp.StatusCode, gzErr)
			}
			defer gz.Close()
			reader = gz
		}
		body, _ := io.ReadAll(reader)
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		reader, err = gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer reader.(*gzip.Reader).Close()
	}

	bodyBytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return bodyBytes, nil
}

type AuthResponse struct {
	Expiry string
	Auth   string
}

func NewApi() (*Api, error) {
	selectedEmail := AppConfig.Selected
	if len(selectedEmail) == 0 {
		return nil, fmt.Errorf("no account is selected")
	}
	credentials := ""
	language := ""
	for _, c := range AppConfig.Credentials {
		params, err := url.ParseQuery(c)
		if err != nil {
			continue
		}
		if params.Get("Email") == selectedEmail {
			credentials = c
			language = params.Get("lang")
		}
	}

	if len(credentials) == 0 {
		return nil, fmt.Errorf("no credentials with matching selcted email found")
	}

	client, err := NewHTTPClientWithProxy(AppConfig.Proxy)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP client: %w", err)
	}

	api := &Api{
		androidAPIVersion: 28,
		model:             "Pixel XL",
		make:              "Google",
		clientVersionCode: 49029607,
		language:          language,
		authData:          strings.TrimSpace(credentials),
		client:            client,
		authResponseCache: map[string]string{
			"Expiry": "0",
			"Auth":   "",
		},
		Email: selectedEmail,
	}

	api.userAgent = fmt.Sprintf(
		"com.google.android.apps.photos/%d (Linux; U; Android 9; %s; %s; Build/PQ2A.190205.001; Cronet/127.0.6510.5) (gzip)",
		api.clientVersionCode,
		api.language,
		api.model,
	)

	return api, nil
}

func buildUserAgent(clientVersionCode int64, language string, model string) string {
	return fmt.Sprintf(
		"com.google.android.apps.photos/%d (Linux; U; Android 9; %s; %s; Build/PQ2A.190205.001; Cronet/127.0.6510.5) (gzip)",
		clientVersionCode,
		language,
		model,
	)
}

func (a *Api) BearerToken() (string, error) {
	expiryStr := a.authResponseCache["Expiry"]
	expiry, err := strconv.ParseInt(expiryStr, 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid expiry time: %w", err)
	}

	if expiry <= time.Now().Unix() {
		resp, err := a.getAuthToken()
		if err != nil {
			return "", fmt.Errorf("failed to get auth token: %w", err)
		}
		a.authResponseCache = resp
	}

	if token, ok := a.authResponseCache["Auth"]; ok && token != "" {
		return token, nil
	}

	return "", errors.New("auth response does not contain bearer token")
}

func (a *Api) getAuthToken() (map[string]string, error) {
	authDataValues, err := url.ParseQuery(a.authData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse auth data: %w", err)
	}

	authRequestData := url.Values{
		"androidId":                    {authDataValues.Get("androidId")},
		"app":                          {"com.google.android.apps.photos"},
		"client_sig":                   {authDataValues.Get("client_sig")},
		"callerPkg":                    {"com.google.android.apps.photos"},
		"callerSig":                    {authDataValues.Get("callerSig")},
		"device_country":               {authDataValues.Get("device_country")},
		"Email":                        {authDataValues.Get("Email")},
		"google_play_services_version": {authDataValues.Get("google_play_services_version")},
		"lang":                         {authDataValues.Get("lang")},
		"oauth2_foreground":            {authDataValues.Get("oauth2_foreground")},
		"sdk_version":                  {authDataValues.Get("sdk_version")},
		"service":                      {authDataValues.Get("service")},
		"Token":                        {authDataValues.Get("Token")},
	}

	headers := map[string]string{
		"Accept-Encoding": "gzip",
		"app":             "com.google.android.apps.photos",
		"Connection":      "Keep-Alive",
		"Content-Type":    "application/x-www-form-urlencoded",
		"device":          authRequestData.Get("androidId"),
		"User-Agent":      "GoogleAuth/1.4 (Pixel XL PQ2A.190205.001); gzip",
	}

	req, err := http.NewRequest(
		"POST",
		"https://android.googleapis.com/auth",
		strings.NewReader(authRequestData.Encode()),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("auth request failed after retries: %w", err)
	}
	defer resp.Body.Close()

	// Check for errors
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return make(map[string]string), fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Handle gzip encoding if present
	var reader io.Reader
	reader, err = gzip.NewReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer reader.(*gzip.Reader).Close()

	// Parse the response body
	bodyBytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse the key=value response format
	parsedAuthResponse := make(map[string]string)
	for _, line := range strings.Split(string(bodyBytes), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			parsedAuthResponse[parts[0]] = parts[1]
		}
	}

	// Validate we got the required fields
	if parsedAuthResponse["Auth"] == "" {
		return nil, errors.New("auth response missing Auth token")
	}
	if parsedAuthResponse["Expiry"] == "" {
		return nil, errors.New("auth response missing Expiry")
	}

	expirySeconds, err := strconv.ParseInt(parsedAuthResponse["Expiry"], 10, 64)
	if err == nil {
		now := time.Now().Unix()
		// The API returns expiry as a relative duration in seconds.
		if expirySeconds < now {
			expirySeconds = now + expirySeconds
		}
		// Refresh a little early to reduce the chance of expired tokens during requests.
		expirySeconds = expirySeconds - 30
		if expirySeconds < now {
			expirySeconds = now
		}
		parsedAuthResponse["Expiry"] = strconv.FormatInt(expirySeconds, 10)
	}

	return parsedAuthResponse, nil
}

// Obtain a file upload token from the Google Photos API.
func (a *Api) GetUploadToken(shaHashB64 string, fileSize int64) (string, error) {
	// Create the protobuf message
	protoBody := generated.GetUploadToken{
		F1:            2,
		F2:            2,
		F3:            1,
		F4:            3,
		FileSizeBytes: fileSize,
	}

	// Serialize the protobuf message
	serializedData, err := proto.Marshal(&protoBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal protobuf: %w", err)
	}

	// Get the bearer token
	bearerToken, err := a.BearerToken()
	if err != nil {
		return "", fmt.Errorf("failed to get bearer token: %w", err)
	}

	// Prepare headers
	headers := map[string]string{
		"Accept-Encoding":         "gzip",
		"Accept-Language":         a.language,
		"Content-Type":            "application/x-protobuf",
		"User-Agent":              a.userAgent,
		"Authorization":           "Bearer " + bearerToken,
		"X-Goog-Hash":             "sha1=" + shaHashB64,
		"X-Upload-Content-Length": strconv.Itoa(int(fileSize)),
	}

	// Create the request
	req, err := http.NewRequest(
		"POST",
		"https://photos.googleapis.com/data/upload/uploadmedia/interactive",
		bytes.NewReader(serializedData),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Make the request
	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check for errors
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Get the upload token from headers
	uploadToken := resp.Header.Get("X-GUploader-UploadID")
	if uploadToken == "" {
		return "", errors.New("response missing X-GUploader-UploadID header")
	}

	return uploadToken, nil
}

// Check library for existing files with the hash
func (a *Api) FindRemoteMediaByHash(shaHash []byte) (string, error) {
	// Create the protobuf message

	// Create and initialize the protobuf message with all required nested structures
	protoBody := generated.HashCheck{
		Field1: &generated.HashCheckField1Type{
			Field1: &generated.HashCheckField1TypeField1Type{
				Sha1Hash: shaHash,
			},
			Field2: &generated.HashCheckField1TypeField2Type{},
		},
	}

	// Serialize the protobuf message
	serializedData, err := proto.Marshal(&protoBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal protobuf: %w", err)
	}

	// Get the bearer token
	bearerToken, err := a.BearerToken()
	if err != nil {
		return "", fmt.Errorf("failed to get bearer token: %w", err)
	}

	// Prepare headers
	headers := map[string]string{
		"Accept-Encoding": "gzip",
		"Accept-Language": a.language,
		"Content-Type":    "application/x-protobuf",
		"User-Agent":      a.userAgent,
		"Authorization":   "Bearer " + bearerToken,
	}

	// Create the request
	req, err := http.NewRequest(
		"POST",
		"https://photosdata-pa.googleapis.com/6439526531001121323/5084965799730810217",
		bytes.NewReader(serializedData),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Make the request
	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check for errors
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var reader io.Reader
	reader, err = gzip.NewReader(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer reader.(*gzip.Reader).Close()

	// Parse the response body
	bodyBytes, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var pbResp generated.RemoteMatches
	if err := proto.Unmarshal(bodyBytes, &pbResp); err != nil {
		log.Fatalf("Failed to unmarshal protobuf: %v", err)
	}

	mediaKey := pbResp.GetMediaKey()

	return mediaKey, nil
}

func (a *Api) UploadFile(ctx context.Context, filePath string, uploadToken string) (*generated.CommitToken, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("error opening file: %w", err)
	}
	defer file.Close()

	uploadURL := "https://photos.googleapis.com/data/upload/uploadmedia/interactive?upload_id=" + uploadToken

	req, err := http.NewRequestWithContext(ctx, "PUT", uploadURL, file)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	// Important: Don't set ContentLength to enable chunked transfer encoding
	req.ContentLength = -1

	bearerToken, err := a.BearerToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get bearer token: %w", err)
	}

	headers := map[string]string{
		"Accept-Encoding": "gzip",
		"Accept-Language": a.language,
		"User-Agent":      a.userAgent,
		"Authorization":   "Bearer " + bearerToken,
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var pbResp generated.CommitToken
	if err := proto.Unmarshal(bodyBytes, &pbResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal protobuf: %w", err)
	}

	return &pbResp, nil
}

// CommitUpload commits the upload to Google Photos
func (a *Api) CommitUpload(
	uploadResponseDecoded *generated.CommitToken,
	fileName string,
	sha1Hash []byte,
	uploadTimestamp int64,
) (string, error) {
	if uploadTimestamp == 0 {
		uploadTimestamp = time.Now().Unix()
	}

	model := a.model
	userAgent := a.userAgent

	var qualityVal int64 = 3
	if AppConfig.Saver {
		qualityVal = 1
		model = "Pixel 2"
		userAgent = buildUserAgent(a.clientVersionCode, a.language, model)
	}

	if AppConfig.UseQuota {
		model = "Pixel 8"
		userAgent = buildUserAgent(a.clientVersionCode, a.language, model)
	}

	unknownInt := int64(46000000)

	// Create the protobuf message
	protoBody := generated.CommitUpload{
		Field1: &generated.CommitUploadField1Type{
			Field1: &generated.CommitUploadField1TypeField1Type{
				Field1: uploadResponseDecoded.Field1,
				Field2: uploadResponseDecoded.Field2,
			},
			FileName: fileName,
			Sha1Hash: sha1Hash,
			Field4: &generated.CommitUploadField1TypeField4Type{
				FileLastModifiedTimestamp: uploadTimestamp,
				Field2:                    unknownInt,
			},
			Quality: qualityVal,
			Field10: 1,
		},
		Field2: &generated.CommitUploadField2Type{
			Model:             model,
			Make:              a.make,
			AndroidApiVersion: a.androidAPIVersion,
		},
		Field3: []byte{1, 3},
	}

	// Serialize the protobuf message
	serializedData, err := proto.Marshal(&protoBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal protobuf: %w", err)
	}

	// Get the bearer token
	bearerToken, err := a.BearerToken()
	if err != nil {
		return "", fmt.Errorf("failed to get bearer token: %w", err)
	}

	// Prepare headers
	headers := map[string]string{
		"accept-Encoding":          "gzip",
		"accept-Language":          a.language,
		"content-Type":             "application/x-protobuf",
		"user-Agent":               userAgent,
		"authorization":            "Bearer " + bearerToken,
		"x-goog-ext-173412678-bin": "CgcIAhClARgC",
		"x-goog-ext-174067345-bin": "CgIIAg==",
	}

	// Create the request
	req, err := http.NewRequest(
		"POST",
		"https://photosdata-pa.googleapis.com/6439526531001121323/16538846908252377752",
		bytes.NewReader(serializedData),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Make the request
	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check for errors
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Handle gzip response if needed
	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		reader, err = gzip.NewReader(resp.Body)
		if err != nil {
			return "", fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer reader.(*gzip.Reader).Close()
	}

	// Parse the response body
	bodyBytes, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var pbResp generated.CommitUploadResponse
	if err := proto.Unmarshal(bodyBytes, &pbResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal protobuf: %w", err)
	}

	// Get media key from response
	if pbResp.GetField1() == nil || pbResp.GetField1().GetField3() == nil {
		return "", fmt.Errorf("upload rejected by API: invalid response structure")
	}

	mediaKey := pbResp.GetField1().GetField3().GetMediaKey()
	if mediaKey == "" {
		return "", fmt.Errorf("upload rejected by API: no media key returned")
	}

	return mediaKey, nil
}

// CommitUploadOverride commits an upload with explicit client model and quality, bypassing AppConfig-based defaults.
// This is used for workflows like "washing" quota-consuming items by re-uploading with a different client profile.
func (a *Api) CommitUploadOverride(
	uploadResponseDecoded *generated.CommitToken,
	fileName string,
	sha1Hash []byte,
	uploadTimestamp int64,
	model string,
	qualityVal int64,
) (string, error) {
	if uploadTimestamp == 0 {
		uploadTimestamp = time.Now().Unix()
	}
	if model == "" {
		model = a.model
	}
	if qualityVal == 0 {
		qualityVal = 3
	}
	userAgent := buildUserAgent(a.clientVersionCode, a.language, model)

	unknownInt := int64(46000000)

	protoBody := generated.CommitUpload{
		Field1: &generated.CommitUploadField1Type{
			Field1: &generated.CommitUploadField1TypeField1Type{
				Field1: uploadResponseDecoded.Field1,
				Field2: uploadResponseDecoded.Field2,
			},
			FileName: fileName,
			Sha1Hash: sha1Hash,
			Field4: &generated.CommitUploadField1TypeField4Type{
				FileLastModifiedTimestamp: uploadTimestamp,
				Field2:                    unknownInt,
			},
			Quality: qualityVal,
			Field10: 1,
		},
		Field2: &generated.CommitUploadField2Type{
			Model:             model,
			Make:              a.make,
			AndroidApiVersion: a.androidAPIVersion,
		},
		Field3: []byte{1, 3},
	}

	serializedData, err := proto.Marshal(&protoBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal protobuf: %w", err)
	}

	bearerToken, err := a.BearerToken()
	if err != nil {
		return "", fmt.Errorf("failed to get bearer token: %w", err)
	}

	headers := map[string]string{
		"accept-Encoding":          "gzip",
		"accept-Language":          a.language,
		"content-Type":             "application/x-protobuf",
		"user-Agent":               userAgent,
		"authorization":            "Bearer " + bearerToken,
		"x-goog-ext-173412678-bin": "CgcIAhClARgC",
		"x-goog-ext-174067345-bin": "CgIIAg==",
	}

	req, err := http.NewRequest(
		"POST",
		"https://photosdata-pa.googleapis.com/6439526531001121323/16538846908252377752",
		bytes.NewReader(serializedData),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		reader, err = gzip.NewReader(resp.Body)
		if err != nil {
			return "", fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer reader.(*gzip.Reader).Close()
	}

	bodyBytes, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var pbResp generated.CommitUploadResponse
	if err := proto.Unmarshal(bodyBytes, &pbResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal protobuf: %w", err)
	}

	if pbResp.GetField1() == nil || pbResp.GetField1().GetField3() == nil {
		return "", fmt.Errorf("upload rejected by API: invalid response structure")
	}

	mediaKey := pbResp.GetField1().GetField3().GetMediaKey()
	if mediaKey == "" {
		return "", fmt.Errorf("upload rejected by API: no media key returned")
	}

	return mediaKey, nil
}

// DownloadURLs contains the download URLs for a media item
type DownloadURLs struct {
	EditedURL   string // URL for downloading the file with applied edits (if any)
	OriginalURL string // URL for downloading the original file
	Filename    string // Original filename of the media item
}

// GetDownloadURLs retrieves download URLs for a media item
func (a *Api) GetDownloadURLs(mediaKey string) (*DownloadURLs, error) {
	// Create the protobuf message
	protoBody := generated.GetDownloadUrls{
		Field1: &generated.GetDownloadUrlsField1Type{
			Field1: &generated.GetDownloadUrlsField1Field1Type{
				MediaKey: mediaKey,
			},
		},
		Field2: &generated.GetDownloadUrlsField2Type{
			Field1: &generated.GetDownloadUrlsField2Field1Type{
				Field7: &generated.GetDownloadUrlsField2Field1Field7Type{
					Field2: &generated.GetDownloadUrlsEmpty{},
				},
			},
			Field5: &generated.GetDownloadUrlsField2Field5Type{
				Field2: &generated.GetDownloadUrlsEmpty{},
				Field3: &generated.GetDownloadUrlsEmpty{},
				Field5: &generated.GetDownloadUrlsField2Field5Field5Type{
					Field1: &generated.GetDownloadUrlsEmpty{},
					Field3: 1,
				},
			},
		},
	}

	// Serialize the protobuf message
	serializedData, err := proto.Marshal(&protoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal protobuf: %w", err)
	}

	// Get the bearer token
	bearerToken, err := a.BearerToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get bearer token: %w", err)
	}

	// Prepare headers
	headers := map[string]string{
		"accept-encoding":          "gzip",
		"Accept-Language":          a.language,
		"Content-Type":             "application/x-protobuf",
		"User-Agent":               a.userAgent,
		"Authorization":            "Bearer " + bearerToken,
		"x-goog-ext-173412678-bin": "CgcIAhClARgC",
		"x-goog-ext-174067345-bin": "CgIIAg==",
	}

	// Create the request
	req, err := http.NewRequest(
		"POST",
		"https://photosdata-pa.googleapis.com/$rpc/social.frontend.photos.preparedownloaddata.v1.PhotosPrepareDownloadDataService/PhotosPrepareDownload",
		bytes.NewReader(serializedData),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Make the request
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check for errors
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Handle gzip response if needed
	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		reader, err = gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer reader.(*gzip.Reader).Close()
	}

	// Parse the response body
	bodyBytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var pbResp generated.GetDownloadUrlsResponse
	if err := proto.Unmarshal(bodyBytes, &pbResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal protobuf: %w", err)
	}

	// Extract URLs and filename from response
	result := &DownloadURLs{}
	if field1 := pbResp.GetField1(); field1 != nil {
		// Extract filename from field2.field4
		if field2 := field1.GetField2(); field2 != nil {
			result.Filename = field2.GetField4()
		}

		// Extract download URLs from field5
		if field5 := field1.GetField5(); field5 != nil {
			// Try to get video download URL first from field3.field5
			// Videos have a different structure than photos
			if field3 := field5.GetField3(); field3 != nil {
				videoURL := field3.GetField5()
				if videoURL != "" {
					// For videos, use the video URL as the original URL
					// Clear both URLs first to avoid mixing video and photo data
					result.OriginalURL = videoURL
					result.EditedURL = ""
					return result, nil
				}
			}

			// If no video URL, try to get photo download URLs from field2
			if field2 := field5.GetField2(); field2 != nil {
				result.EditedURL = field2.GetEditedUrl()
				result.OriginalURL = field2.GetOriginalUrl()
			}
		}
	}

	return result, nil
}

// GetMediaInfo retrieves metadata for a specific media item by its media key
// This includes the filename and other metadata
func (a *Api) GetMediaInfo(mediaKey string) (*MediaItem, error) {
	// Build the request to get media info for a specific media key
	requestData := buildGetMediaInfoRequest(mediaKey)

	// Get the bearer token
	bearerToken, err := a.BearerToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get bearer token: %w", err)
	}

	// Prepare headers
	headers := map[string]string{
		"accept-encoding":          "gzip",
		"Accept-Language":          a.language,
		"Content-Type":             "application/x-protobuf",
		"User-Agent":               a.userAgent,
		"Authorization":            "Bearer " + bearerToken,
		"x-goog-ext-173412678-bin": "CgcIAhClARgC",
		"x-goog-ext-174067345-bin": "CgIIAg==",
	}

	// Create the request
	req, err := http.NewRequest(
		"POST",
		"https://photosdata-pa.googleapis.com/6439526531001121323/18047484249733410717",
		bytes.NewReader(requestData),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Make the request
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check for errors
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Handle gzip response if needed
	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		reader, err = gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer reader.(*gzip.Reader).Close()
	}

	// Read the response body
	bodyBytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse the response to extract media item info
	item := parseMediaInfoResponse(bodyBytes, mediaKey)
	if item == nil {
		return nil, fmt.Errorf("media item not found for key: %s", mediaKey)
	}

	return item, nil
}

const moveToTrashEndpoint = "https://photosdata-pa.googleapis.com/6439526531001121323/17490284929287180316"

// MoveToTrash moves media items to trash (soft delete).
// dedupKeys should be the mediaKey strings used in list responses.
func (a *Api) MoveToTrash(dedupKeys []string) error {
	if len(dedupKeys) == 0 {
		return fmt.Errorf("no keys provided")
	}

	keys := make([]string, 0, len(dedupKeys))
	for _, k := range dedupKeys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return fmt.Errorf("no valid keys provided")
	}

	requestData := buildMoveToTrashRequest(keys, a.clientVersionCode, a.androidAPIVersion)

	bearerToken, err := a.BearerToken()
	if err != nil {
		return fmt.Errorf("failed to get bearer token: %w", err)
	}

	headers := map[string]string{
		"Accept-Encoding": "gzip",
		"Accept-Language": a.language,
		"Content-Type":    "application/x-protobuf",
		"User-Agent":      a.userAgent,
		"Authorization":   "Bearer " + bearerToken,
		"x-goog-ext-173412678-bin": "CgcIAhClARgC",
		"x-goog-ext-174067345-bin": "CgIIAg==",
	}

	req, err := http.NewRequest("POST", moveToTrashEndpoint, bytes.NewReader(requestData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var reader io.Reader = resp.Body
		if resp.Header.Get("Content-Encoding") == "gzip" {
			gz, gzErr := gzip.NewReader(resp.Body)
			if gzErr != nil {
				return fmt.Errorf("request failed with status %d (gzip reader error: %v)", resp.StatusCode, gzErr)
			}
			defer gz.Close()
			reader = gz
		}
		body, _ := io.ReadAll(reader)
		return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Success: the current implementation treats any 2xx as OK and ignores response protobuf.
	return nil
}

func buildMoveToTrashRequest(dedupKeys []string, clientVersionCode int64, androidAPIVersion int64) []byte {
	var buf bytes.Buffer

	// Field 2: operation type = 1 (move to trash)
	writeProtobufVarint(&buf, 2, 1)

	// Field 3: repeated item keys (mediaKey strings)
	for _, k := range dedupKeys {
		writeProtobufString(&buf, 3, k)
	}

	// Field 4: operation mode = 1
	writeProtobufVarint(&buf, 4, 1)

	// Field 8: fixed nested meta structure
	var field8 bytes.Buffer
	var field8_4 bytes.Buffer
	writeProtobufField(&field8_4, 2, []byte{}) // 8.4.2 = {}
	var field8_4_3 bytes.Buffer
	writeProtobufField(&field8_4_3, 1, []byte{}) // 8.4.3.1 = {}
	writeProtobufField(&field8_4, 3, field8_4_3.Bytes())
	writeProtobufField(&field8_4, 4, []byte{}) // 8.4.4 = {}
	var field8_4_5 bytes.Buffer
	writeProtobufField(&field8_4_5, 1, []byte{}) // 8.4.5.1 = {}
	writeProtobufField(&field8_4, 5, field8_4_5.Bytes())
	writeProtobufField(&field8, 4, field8_4.Bytes())
	writeProtobufField(&buf, 8, field8.Bytes())

	// Field 9: client info
	var field9 bytes.Buffer
	writeProtobufVarint(&field9, 1, 5) // 9.1 = 5
	var field9_2 bytes.Buffer
	writeProtobufVarint(&field9_2, 1, clientVersionCode)                    // 9.2.1
	writeProtobufString(&field9_2, 2, fmt.Sprintf("%d", androidAPIVersion)) // 9.2.2
	writeProtobufField(&field9, 2, field9_2.Bytes())
	writeProtobufField(&buf, 9, field9.Bytes())

	return buf.Bytes()
}

// buildGetMediaInfoRequest creates a protobuf request to get info for a specific media key
func buildGetMediaInfoRequest(mediaKey string) []byte {
	var buf bytes.Buffer

	// Build field 1 (request data)
	field1 := buildGetMediaInfoRequestField1(mediaKey)
	writeProtobufField(&buf, 1, field1)

	// Build field 2 (additional options)
	field2 := buildMediaListRequestField2()
	writeProtobufField(&buf, 2, field2)

	return buf.Bytes()
}

func buildGetMediaInfoRequestField1(mediaKey string) []byte {
	var buf bytes.Buffer

	// field1.1 - media metadata options (file info, timestamps, etc.)
	// Structure:
	// 1.1:
	//   1: { 19: "", 20: "", 25: "", 30: { 2: "" } }
	//   3..41: ""
	//   21: { 1: <mode>, 5: { 3: "" } }

	var field1_1 bytes.Buffer

	// 1.1.1
	var field1_1_1 bytes.Buffer
	writeProtobufString(&field1_1_1, 19, "")
	writeProtobufString(&field1_1_1, 20, "")
	writeProtobufString(&field1_1_1, 25, "")
	
	var field1_1_1_30 bytes.Buffer
	writeProtobufString(&field1_1_1_30, 2, "")
	writeProtobufField(&field1_1_1, 30, field1_1_1_30.Bytes())
	
	writeProtobufField(&field1_1, 1, field1_1_1.Bytes())

	// 1.1.x (simple strings)
	simpleFields := []int{3, 4, 5, 6, 7, 15, 16, 17, 19, 20, 25, 30, 31, 32, 33, 34, 36, 37, 38, 39, 40, 41}
	for _, f := range simpleFields {
		writeProtobufString(&field1_1, f, "")
	}

	// 1.1.21 (dedup key request)
	trashMode := int64(2)
	if !AppConfig.RequestTrashItems {
		trashMode = 1
	}
	var field1_1_21 bytes.Buffer
	writeProtobufVarint(&field1_1_21, 1, trashMode)
	var field1_1_21_5 bytes.Buffer
	writeProtobufString(&field1_1_21_5, 3, "")
	writeProtobufField(&field1_1_21, 5, field1_1_21_5.Bytes())
	writeProtobufField(&field1_1, 21, field1_1_21.Bytes())

	writeProtobufField(&buf, 1, field1_1.Bytes())

	// field1.3 - album and collection options
	albumOptions := []int{2, 3, 7, 8, 14, 16, 17, 18, 19, 20, 21, 22, 23, 27, 29, 30, 31, 32, 34, 37, 38, 39, 41}
	field1_3 := buildEmptyNestedMessage(albumOptions)
	writeProtobufField(&buf, 3, field1_3)

	// field1.5 - media key filter
	var field5 bytes.Buffer
	writeProtobufString(&field5, 1, mediaKey)
	writeProtobufField(&buf, 5, field5.Bytes())

	// field1.7 - type (varint = 2)
	writeProtobufVarint(&buf, 7, 2)

	// field1.11 - repeated ints [1, 2]
	writeProtobufVarint(&buf, 11, 1)
	writeProtobufVarint(&buf, 11, 2)

	// field1.22 - some config
	var field22 bytes.Buffer
	writeProtobufVarint(&field22, 1, 2)
	writeProtobufField(&buf, 22, field22.Bytes())

	return buf.Bytes()
}

// selectBetterItem compares two media items and returns the better one
// Prefers items with filename, otherwise returns the new item if current is nil
func selectBetterItem(current, candidate *MediaItem) *MediaItem {
	if candidate == nil {
		return current
	}
	// If candidate has filename and current doesn't, prefer candidate
	if candidate.Filename != "" {
		if current == nil || current.Filename == "" {
			return candidate
		}
	}
	// If current is nil, use candidate
	if current == nil {
		return candidate
	}
	return current
}

// parseMediaInfoResponse parses the protobuf response to extract media item info
// for the target media key. Returns nil if no matching item is found.
func parseMediaInfoResponse(data []byte, targetMediaKey string) *MediaItem {
	// Parse the response using the same logic as media list parsing
	items, _, _ := extractMediaItemsFromResponse(data)

	// Find the matching item (prefer ones with filename)
	var matchedItem *MediaItem
	for i := range items {
		if items[i].MediaKey == targetMediaKey {
			candidate := &items[i]
			if candidate.Filename != "" {
				// Found a match with filename, return immediately
				return candidate
			}
			matchedItem = selectBetterItem(matchedItem, candidate)
		}
	}

	// If we found a match (even without filename), return it
	if matchedItem != nil {
		return matchedItem
	}

	// If not found in standard parsing, try to extract from nested structures
	return tryExtractMediaItem(data, targetMediaKey)
}

// tryExtractMediaItem attempts to extract media item info from the response data
// It recursively searches nested structures for the target media key
func tryExtractMediaItem(data []byte, targetMediaKey string) *MediaItem {
	var result *MediaItem

	offset := 0
	for offset < len(data) {
		fieldNum, wireType, newOffset := readTag(data, offset)
		if newOffset < 0 {
			break
		}
		offset = newOffset

		switch wireType {
		case 0: // Varint
			_, newOffset := readVarint(data, offset)
			if newOffset < 0 {
				return result
			}
			offset = newOffset
		case 2: // Length-delimited
			length, newOffset := readVarint(data, offset)
			if newOffset < 0 || newOffset+int(length) > len(data) {
				return result
			}
			fieldData := data[newOffset : newOffset+int(length)]
			offset = newOffset + int(length)

			// Try to parse this field as a media item
			if fieldNum == 1 || fieldNum == 2 {
				item := tryParseMediaItemWithKey(fieldData, targetMediaKey)
				if item != nil && item.MediaKey == targetMediaKey {
					if item.Filename != "" {
						return item
					}
					result = selectBetterItem(result, item)
				}
				// Recurse into nested messages
				nested := tryExtractMediaItem(fieldData, targetMediaKey)
				if nested != nil && nested.MediaKey == targetMediaKey {
					if nested.Filename != "" {
						return nested
					}
					result = selectBetterItem(result, nested)
				}
			}
		case 5: // 32-bit
			if offset+4 > len(data) {
				return result
			}
			offset += 4
		case 1: // 64-bit
			if offset+8 > len(data) {
				return result
			}
			offset += 8
		case 3: // Start group
			newOffset, ok := skipGroup(data, offset, fieldNum)
			if !ok {
				return result
			}
			offset = newOffset
		case 4: // End group
			return result
		default:
			newOffset, ok := skipField(data, wireType, offset, fieldNum)
			if !ok {
				return result
			}
			offset = newOffset
		}
	}

	return result
}

// PermanentlyDelete permanently deletes media items by dedup key (2.21.1).
func (a *Api) PermanentlyDelete(dedupKeys []string) error {
	if len(dedupKeys) == 0 {
		return fmt.Errorf("no keys provided")
	}

	keys := make([]string, 0, len(dedupKeys))
	for _, k := range dedupKeys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return fmt.Errorf("no valid keys provided")
	}

	requestData := buildPermanentlyDeleteRequest(keys)

	bearerToken, err := a.BearerToken()
	if err != nil {
		return fmt.Errorf("failed to get bearer token: %w", err)
	}

	headers := map[string]string{
		"Accept-Encoding":          "gzip",
		"Accept-Language":          a.language,
		"Content-Type":             "application/x-protobuf",
		"User-Agent":               a.userAgent,
		"Authorization":            "Bearer " + bearerToken,
		"x-goog-ext-173412678-bin": "CgcIAhClARgC",
		"x-goog-ext-174067345-bin": "CgIIAg==",
	}

	req, err := http.NewRequest("POST", moveToTrashEndpoint, bytes.NewReader(requestData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var reader io.Reader = resp.Body
		if resp.Header.Get("Content-Encoding") == "gzip" {
			gz, gzErr := gzip.NewReader(resp.Body)
			if gzErr != nil {
				return fmt.Errorf("request failed with status %d (gzip reader error: %v)", resp.StatusCode, gzErr)
			}
			defer gz.Close()
			reader = gz
		}
		body, _ := io.ReadAll(reader)
		return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// buildPermanentlyDeleteRequest builds the protobuf request described by:
// {
//   "2": 2,
//   "3": "<dedupKey>",
//   "4": 2,
//   "8": { "4": { "2": "", "3": { "1": "" }, "4": "", "5": { "1": "" } } },
//   "9": ""
// }
func buildPermanentlyDeleteRequest(dedupKeys []string) []byte {
	var buf bytes.Buffer

	// Field 2: operation type = 2 (permanent delete)
	writeProtobufVarint(&buf, 2, 2)

	// Field 3: repeated item keys (dedup keys)
	for _, k := range dedupKeys {
		writeProtobufString(&buf, 3, k)
	}

	// Field 4: operation mode = 2
	writeProtobufVarint(&buf, 4, 2)

	// Field 8: fixed nested meta structure (same shape as trash request)
	var field8 bytes.Buffer
	var field8_4 bytes.Buffer
	writeProtobufField(&field8_4, 2, []byte{}) // 8.4.2 = ""
	var field8_4_3 bytes.Buffer
	writeProtobufField(&field8_4_3, 1, []byte{}) // 8.4.3.1 = ""
	writeProtobufField(&field8_4, 3, field8_4_3.Bytes())
	writeProtobufField(&field8_4, 4, []byte{}) // 8.4.4 = ""
	var field8_4_5 bytes.Buffer
	writeProtobufField(&field8_4_5, 1, []byte{}) // 8.4.5.1 = ""
	writeProtobufField(&field8_4, 5, field8_4_5.Bytes())
	writeProtobufField(&field8, 4, field8_4.Bytes())
	writeProtobufField(&buf, 8, field8.Bytes())

	// Field 9: present as empty string in captured request
	writeProtobufString(&buf, 9, "")

	return buf.Bytes()
}

// tryParseMediaItemWithKey parses a message that might contain a media item with the target key
func tryParseMediaItemWithKey(data []byte, targetMediaKey string) *MediaItem {
	item := &MediaItem{CountsTowardsQuota: false}

	offset := 0
	for offset < len(data) {
		fieldNum, wireType, newOffset := readTag(data, offset)
		if newOffset < 0 {
			break
		}
		offset = newOffset

		switch wireType {
		case 0: // Varint
			val, newOffset := readVarint(data, offset)
			if newOffset < 0 {
				return item
			}
			offset = newOffset
			if fieldNum == 5 {
				if val == 1 {
					item.MediaType = "photo"
				} else if val == 2 {
					item.MediaType = "video"
				}
			}
		case 2: // Length-delimited
			length, newOffset := readVarint(data, offset)
			if newOffset < 0 || newOffset+int(length) > len(data) {
				return item
			}
			fieldData := data[newOffset : newOffset+int(length)]
			offset = newOffset + int(length)

			switch fieldNum {
			case 1:
				// Could be media key (string) or nested message
				if isPrintableString(fieldData) && len(fieldData) > minMediaKeyLength {
					item.MediaKey = string(fieldData)
				} else {
					// Try to parse nested message
					nested := tryParseMediaItemWithKey(fieldData, targetMediaKey)
					if nested != nil {
						if item.MediaKey == "" && nested.MediaKey != "" {
							item.MediaKey = nested.MediaKey
						}
						if item.Filename == "" && nested.Filename != "" {
							item.Filename = nested.Filename
						}
						if item.MediaType == "" && nested.MediaType != "" {
							item.MediaType = nested.MediaType
						}
						if item.DedupKey == "" && nested.DedupKey != "" {
							item.DedupKey = nested.DedupKey
						}
					}
				}
			case 2:
				// Field 2 contains nested metadata with filename at sub-field 4
				filename, countsTowardsQuota, _, isTrash := extractField2Metadata(fieldData)
				if filename != "" {
					item.Filename = filename
				} else if isPrintableString(fieldData) {
					// Could be dedup key or filename
					str := string(fieldData)
					if strings.Contains(str, ".") && item.Filename == "" {
						item.Filename = str
					} else if item.DedupKey == "" {
						item.DedupKey = str
					}
				}

				if countsTowardsQuota {
					item.CountsTowardsQuota = true
				}
				if isTrash {
					item.IsTrash = true
				}
				if item.DedupKey == "" {
					item.DedupKey = extractDedupKeyFromField2(fieldData)
				}
			case 6:
				// Field 6 is often a nested message that also contains the media key at sub-field 1
				if item.MediaKey == "" {
					nested := tryParseMediaItem(fieldData)
					if nested != nil && nested.MediaKey != "" {
						item.MediaKey = nested.MediaKey
					}
				}
			}
		case 5: // 32-bit
			if offset+4 > len(data) {
				return item
			}
			offset += 4
		case 1: // 64-bit
			if offset+8 > len(data) {
				return item
			}
			offset += 8
		case 3: // Start group
			newOffset, ok := skipGroup(data, offset, fieldNum)
			if !ok {
				return item
			}
			offset = newOffset
		case 4: // End group
			return item
		default:
			newOffset, ok := skipField(data, wireType, offset, fieldNum)
			if !ok {
				return item
			}
			offset = newOffset
		}

		// Field 22 indicates quota usage (at item level)
		if fieldNum == 22 {
			item.CountsTowardsQuota = true
		}
	}

	return item
}

// extractField2Metadata extracts the filename, quota usage hint, and status from field 2 of a media item
// Based on the structure: field2 -> field4 = filename, field2 -> field22 = quota consumption marker
// field2 -> field16 -> field1 = status (1=Add, 2=Delete)
func extractField2Metadata(data []byte) (string, bool, int, bool) {
	offset := 0
	filename := ""
	countsTowardsQuota := false
	status := 0
	isTrash := false
	for offset < len(data) {
		fieldNum, wireType, newOffset := readTag(data, offset)
		if newOffset < 0 {
			break
		}
		offset = newOffset

		switch wireType {
		case 0: // Varint
			val, newOffset := readVarint(data, offset)
			offset = newOffset
			if fieldNum == 26 && val == 1096 {
				isTrash = true
			}
		case 2: // Length-delimited
			length, newOffset := readVarint(data, offset)
			if newOffset < 0 || newOffset+int(length) > len(data) {
				return filename, countsTowardsQuota, status, isTrash
			}
			fieldData := data[newOffset : newOffset+int(length)]
			offset = newOffset + int(length)

			// Field 2 nested message (recursion)
			if fieldNum == 2 {
				fName, cQuota, s, iTrash := extractField2Metadata(fieldData)
				if fName != "" && filename == "" {
					filename = fName
				}
				if cQuota {
					countsTowardsQuota = true
				}
				if s > 0 {
					status = s
				}
				if iTrash {
					isTrash = true
				}
			}

			// Field 4 is the filename
			if fieldNum == 4 && isPrintableString(fieldData) {
				filename = string(fieldData)
			}

			// Field 16 contains status info
			if fieldNum == 16 {
				// Parse field 16 (nested message) to find field 1 (status)
				s := parseStatusField(fieldData)
				if s > 0 {
					status = s
					if status == 2 {
						isTrash = true
					}
				}
			}

			if fieldNum == 22 {
				if parseQuotaInfo(fieldData) {
					countsTowardsQuota = true
				}
			}
		case 5: // 32-bit
			offset += 4
		case 1: // 64-bit
			offset += 8
		default:
			return filename, countsTowardsQuota, status, isTrash
		}
	}
	return filename, countsTowardsQuota, status, isTrash
}

func extractDedupKeyFromField21(data []byte) string {
	offset := 0
	for offset < len(data) {
		fieldNum, wireType, newOffset := readTag(data, offset)
		if newOffset < 0 {
			return ""
		}
		offset = newOffset

		switch wireType {
		case 0:
			_, newOffset := readVarint(data, offset)
			if newOffset < 0 {
				return ""
			}
			offset = newOffset
		case 2:
			length, newOffset := readVarint(data, offset)
			if newOffset < 0 || newOffset+int(length) > len(data) {
				return ""
			}
			fieldData := data[newOffset : newOffset+int(length)]
			offset = newOffset + int(length)
			// 21.1 = dedup key string
			if fieldNum == 1 && isPrintableString(fieldData) {
				return string(fieldData)
			}
		case 5:
			if offset+4 > len(data) {
				return ""
			}
			offset += 4
		case 1:
			if offset+8 > len(data) {
				return ""
			}
			offset += 8
		case 3:
			newOffset, ok := skipGroup(data, offset, fieldNum)
			if !ok {
				return ""
			}
			offset = newOffset
		case 4:
			return ""
		default:
			newOffset, ok := skipField(data, wireType, offset, fieldNum)
			if !ok {
				return ""
			}
			offset = newOffset
		}
	}
	return ""
}

// extractDedupKeyFromField2 extracts field 2.21.1 (dedup key) from the media item metadata message.
func extractDedupKeyFromField2(data []byte) string {
	offset := 0
	for offset < len(data) {
		fieldNum, wireType, newOffset := readTag(data, offset)
		if newOffset < 0 {
			return ""
		}
		offset = newOffset

		switch wireType {
		case 0:
			_, newOffset := readVarint(data, offset)
			if newOffset < 0 {
				return ""
			}
			offset = newOffset
		case 2:
			length, newOffset := readVarint(data, offset)
			if newOffset < 0 || newOffset+int(length) > len(data) {
				return ""
			}
			fieldData := data[newOffset : newOffset+int(length)]
			offset = newOffset + int(length)

			// 2.21 = nested message containing the dedup key at 2.21.1
			if fieldNum == 21 {
				if k := extractDedupKeyFromField21(fieldData); k != "" {
					return k
				}
			}

			// field 2 sometimes nests itself; recurse.
			if fieldNum == 2 {
				if k := extractDedupKeyFromField2(fieldData); k != "" {
					return k
				}
			}
		case 5:
			if offset+4 > len(data) {
				return ""
			}
			offset += 4
		case 1:
			if offset+8 > len(data) {
				return ""
			}
			offset += 8
		case 3:
			newOffset, ok := skipGroup(data, offset, fieldNum)
			if !ok {
				return ""
			}
			offset = newOffset
		case 4:
			return ""
		default:
			newOffset, ok := skipField(data, wireType, offset, fieldNum)
			if !ok {
				return ""
			}
			offset = newOffset
		}
	}
	return ""
}

// parseStatusField extracts the status from field 16
func parseStatusField(data []byte) int {
	offset := 0
	for offset < len(data) {
		fieldNum, wireType, newOffset := readTag(data, offset)
		if newOffset < 0 {
			break
		}
		offset = newOffset

		if wireType == 0 && fieldNum == 1 {
			val, _ := readVarint(data, offset)
			return int(val)
		}

		switch wireType {
		case 0:
			_, offset = readVarint(data, offset)
		case 2:
			length, newOffset := readVarint(data, offset)
			if newOffset < 0 {
				return 0
			}
			offset = newOffset + int(length)
		case 5:
			offset += 4
		case 1:
			offset += 8
		default:
			return 0
		}
	}
	return 0
}

// GetThumbnail retrieves a thumbnail for a media item
func (a *Api) GetThumbnail(mediaKey string, width, height int, forceJPEG bool, contentVersion int, noOverlay bool) ([]byte, error) {
	bearerToken, err := a.BearerToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get bearer token: %w", err)
	}

	// Build URL
	url := fmt.Sprintf("https://ap2.googleusercontent.com/gpa/%s=k-sg", mediaKey)
	if width > 0 {
		url += fmt.Sprintf("-w%d", width)
	}
	if height > 0 {
		url += fmt.Sprintf("-h%d", height)
	}
	if forceJPEG {
		url += "-rj"
	}
	if contentVersion > 0 {
		url += fmt.Sprintf("-iv%d", contentVersion)
	}
	if noOverlay {
		url += "-no"
	}

	// Prepare headers
	headers := map[string]string{
		"Authorization":   "Bearer " + bearerToken,
		"User-Agent":      a.userAgent,
		"Accept-Encoding": "gzip",
	}

	// Create the request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Make the request
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check for errors
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Handle gzip response if needed
	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		reader, err = gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer reader.(*gzip.Reader).Close()
	}

	// Read the response body
	bodyBytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return bodyBytes, nil
}

// DownloadFile downloads a file from a given URL and saves it to the specified path
func (a *Api) DownloadFile(downloadURL, outputPath string) error {
	bearerToken, err := a.BearerToken()
	if err != nil {
		return fmt.Errorf("failed to get bearer token: %w", err)
	}

	// Prepare headers
	headers := map[string]string{
		"Authorization":   "Bearer " + bearerToken,
		"User-Agent":      a.userAgent,
		"Accept-Encoding": "gzip",
	}

	// Create the request
	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Make the request
	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check for errors
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Create output file
	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	// Handle gzip response if needed
	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		reader, err = gzip.NewReader(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer reader.(*gzip.Reader).Close()
	}

	// Copy response body to file
	_, err = io.Copy(outFile, reader)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// MediaItem represents a media item in the library
type MediaItem struct {
	MediaKey  string `json:"mediaKey"`
	DedupKey  string `json:"dedupKey,omitempty"`
	Filename  string `json:"filename,omitempty"`
	MediaType string `json:"mediaType,omitempty"` // "photo" or "video"
	Timestamp int64  `json:"timestamp,omitempty"`
	// CountsTowardsQuota indicates whether the item consumes storage quota.
	// Field 22 in the response marks items that do consume quota; items without it are treated as quota-exempt.
	CountsTowardsQuota bool `json:"countsTowardsQuota"`
	Status             int  `json:"status,omitempty"` // 1=Add, 2=Remove/Update
	IsTrash            bool `json:"isTrash,omitempty"`
}

// MediaListResult contains the result of a media list request
type MediaListResult struct {
	Items         []MediaItem `json:"items"`
	NextPageToken string      `json:"nextPageToken,omitempty"` // Pagination token from response field 1.1
	SyncToken     string      `json:"syncToken,omitempty"`     // Sync token from response field 1.6
}

// AlbumItem represents a single album in Google Photos
type AlbumItem struct {
	AlbumKey   string `json:"albumKey"`
	Title      string `json:"title,omitempty"`
	MediaCount int    `json:"mediaCount,omitempty"`
}

// AlbumListResult contains the result of an album list request
type AlbumListResult struct {
	Albums        []AlbumItem `json:"albums"`
	NextPageToken string      `json:"nextPageToken,omitempty"` // Pagination token from response field 1.4
}

// minMediaKeyLength is the minimum expected length for a valid media key string
// Google Photos media keys are typically base64-encoded identifiers > 10 chars
const minMediaKeyLength = 10

// GetMediaList retrieves a list of media items from the library
// This uses a simplified request to fetch media items with pagination support
// pageToken should be passed from previous responses (field 1.1) for proper pagination
// syncToken should be passed for incremental updates (field 1.6)
// triggerMode controls the update mode (1=Active/Fetch Changes, 2=Passive/Scan)
func (a *Api) GetMediaList(pageToken string, syncToken string, triggerMode int, limit int) (*MediaListResult, error) {
	// Build the request using raw protobuf wire format
	// The request structure is complex, so we use a helper to build it
	requestData := buildMediaListRequest(pageToken, syncToken, triggerMode, limit)

	// Get the bearer token
	bearerToken, err := a.BearerToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get bearer token: %w", err)
	}

	// Prepare headers
	headers := map[string]string{
		"accept-encoding":          "gzip",
		"Accept-Language":          a.language,
		"Content-Type":             "application/x-protobuf",
		"User-Agent":               a.userAgent,
		"Authorization":            "Bearer " + bearerToken,
		"x-goog-ext-173412678-bin": "CgcIAhClARgC",
		"x-goog-ext-174067345-bin": "CgIIAg==",
	}

	// Create the request
	req, err := http.NewRequest(
		"POST",
		"https://photosdata-pa.googleapis.com/6439526531001121323/18047484249733410717",
		bytes.NewReader(requestData),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Make the request
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check for errors
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Handle gzip response if needed
	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		reader, err = gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer reader.(*gzip.Reader).Close()
	}

	// Read the response body
	bodyBytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse the response to extract media items
	result, err := parseMediaListResponse(bodyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

// buildMediaListRequest creates the protobuf request for fetching media list
// pageToken comes from the previous response's field 1.1 and goes into request field 1.4
func buildMediaListRequest(pageToken string, syncToken string, triggerMode int, limit int) []byte {
	if req, err := buildMediaListRequestFromTemplate(pageToken, syncToken, triggerMode, limit); err == nil && len(req) > 0 {
		return req
	}
	return buildMediaListRequestLegacy(pageToken, syncToken, triggerMode, limit)
}

func buildMediaListRequestFromTemplate(pageToken string, syncToken string, triggerMode int, limit int) ([]byte, error) {
	base, err := getMediaListTemplate()
	if err != nil {
		return nil, err
	}

	rootAny := deepCopyJSON(base)
	root, ok := rootAny.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("template root is not an object")
	}

	field1Any, ok := root["1"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("template missing field 1 object")
	}

	// Dynamic: 1.2 = limit (the new template uses field 1.2 as the page size)
	if limit > 0 {
		field1Any["2"] = int64(limit)
	}

	// Dynamic: 1.4 page token (optional)
	if pageToken != "" {
		field1Any["4"] = pageToken
	} else {
		delete(field1Any, "4")
	}

	// Dynamic: 1.6 sync token (can be empty)
	field1Any["6"] = syncToken

	// Dynamic: 1.22.1 trigger mode
	tMode := int64(2)
	if triggerMode == 1 {
		tMode = 1
	}
	field22, err := ensureMapPath(field1Any, "22")
	if err != nil {
		return nil, err
	}
	field22["1"] = tMode

	return buildProtobufFromMap(root)
}

func buildMediaListRequestLegacy(pageToken string, syncToken string, triggerMode int, limit int) []byte {
	var buf bytes.Buffer

	// Build field 1 (request data)
	field1 := buildMediaListRequestField1(pageToken, syncToken, triggerMode, limit)
	writeProtobufField(&buf, 1, field1)

	// Build field 2 (additional options)
	field2 := buildMediaListRequestField2()
	writeProtobufField(&buf, 2, field2)

	return buf.Bytes()
}

func buildMediaListRequestField1(pageToken string, syncToken string, triggerMode int, limit int) []byte {
	var buf bytes.Buffer

	// These field numbers correspond to the Google Photos protobuf schema for media list requests
	// They define which metadata fields to include in the response
	// field1.1 - media metadata options (file info, timestamps, etc.)
	mediaMetadataFields := []int{1, 3, 4, 5, 6, 7, 15, 16, 17, 19, 20, 21, 25, 30, 31, 32, 33, 34, 36, 37, 38, 39, 40, 41}
	trashMode := int64(2)
	if !AppConfig.RequestTrashItems {
		trashMode = 1
	}
	field1_1 := buildMediaListMetadataOptions(mediaMetadataFields, trashMode)
	writeProtobufField(&buf, 1, field1_1)

	// field1.2 - page size limit (varint)
	if limit > 0 {
		writeProtobufVarint(&buf, 2, int64(limit))
	}

	// field1.3 - album and collection options
	albumOptions := []int{2, 3, 7, 8, 14, 16, 17, 18, 19, 20, 21, 22, 23, 27, 29, 30, 31, 32, 34, 37, 38, 39, 41}
	field1_3 := buildEmptyNestedMessage(albumOptions)
	writeProtobufField(&buf, 3, field1_3)

	// field1.4 - pagination token (string) - this is the only field that changes between requests
	// The value comes from the previous response's field 1.1
	if pageToken != "" {
		writeProtobufString(&buf, 4, pageToken)
	}

	// field1.6 - sync token (string) - for incremental updates
	// The value comes from the previous response's field 1.6
	if syncToken != "" {
		writeProtobufString(&buf, 6, syncToken)
	}

	// field1.7 - type (varint = 2)
	writeProtobufVarint(&buf, 7, 2)

	// field1.11 - repeated ints [1, 2]
	writeProtobufVarint(&buf, 11, 1)
	writeProtobufVarint(&buf, 11, 2)

	// field1.22 - some config including Trigger Mode
	// 1.22.1: Trigger Mode (1=Active, 2=Passive)
	var field22 bytes.Buffer
	tMode := int64(2)
	if triggerMode == 1 {
		tMode = 1
	}
	writeProtobufVarint(&field22, 1, tMode)
	writeProtobufField(&buf, 22, field22.Bytes())

	return buf.Bytes()
}

func buildMediaListMetadataOptions(fields []int, trashMode int64) []byte {
	// Target shape:
	// 1.1 = { 1: { 1: { <fields> } } }
	var inner bytes.Buffer

	for _, f := range fields {
		if f == 21 {
			continue
		}
		writeProtobufString(&inner, f, "")
	}

	// field 21 - nested message (override)
	// 21: { 1: <1|2>, 5: { 3: "" } }
	var field21 bytes.Buffer
	writeProtobufVarint(&field21, 1, trashMode)
	var field21_5 bytes.Buffer
	writeProtobufString(&field21_5, 3, "")
	writeProtobufField(&field21, 5, field21_5.Bytes())
	writeProtobufField(&inner, 21, field21.Bytes())

	var level2 bytes.Buffer
	writeProtobufField(&level2, 1, inner.Bytes())
	var level1 bytes.Buffer
	writeProtobufField(&level1, 1, level2.Bytes())
	return level1.Bytes()
}

func buildMediaListRequestField2() []byte {
	var buf bytes.Buffer
	// Empty nested structure for field 2
	var field2_1 bytes.Buffer
	var field2_1_1 bytes.Buffer
	var field2_1_1_1 bytes.Buffer
	writeProtobufField(&field2_1_1_1, 1, []byte{})
	writeProtobufField(&field2_1_1, 1, field2_1_1_1.Bytes())
	writeProtobufField(&field2_1_1, 2, []byte{})
	writeProtobufField(&field2_1, 1, field2_1_1.Bytes())
	writeProtobufField(&buf, 1, field2_1.Bytes())
	writeProtobufField(&buf, 2, []byte{})
	return buf.Bytes()
}

func buildEmptyNestedMessage(fields []int) []byte {
	var buf bytes.Buffer
	for _, f := range fields {
		writeProtobufField(&buf, f, []byte{})
	}
	return buf.Bytes()
}

// writeProtobufField writes a length-delimited protobuf field
func writeProtobufField(buf *bytes.Buffer, fieldNum int, data []byte) {
	// Wire type 2 (length-delimited)
	tag := (fieldNum << 3) | 2
	writeVarint(buf, uint64(tag))
	writeVarint(buf, uint64(len(data)))
	buf.Write(data)
}

// writeProtobufVarint writes a varint protobuf field
func writeProtobufVarint(buf *bytes.Buffer, fieldNum int, value int64) {
	// Wire type 0 (varint)
	tag := (fieldNum << 3) | 0
	writeVarint(buf, uint64(tag))
	writeVarint(buf, uint64(value))
}

// writeProtobufString writes a string protobuf field
func writeProtobufString(buf *bytes.Buffer, fieldNum int, value string) {
	writeProtobufField(buf, fieldNum, []byte(value))
}

// writeVarint writes a varint to the buffer
func writeVarint(buf *bytes.Buffer, v uint64) {
	for v >= 0x80 {
		buf.WriteByte(byte(v) | 0x80)
		v >>= 7
	}
	buf.WriteByte(byte(v))
}

// parseMediaListResponse parses the protobuf response and extracts media items
func parseMediaListResponse(data []byte) (*MediaListResult, error) {
	result := &MediaListResult{
		Items: []MediaItem{},
	}

	// Parse the response using low-level protobuf parsing
	// The response has a complex structure, we need to navigate to the media items
	items, paginationToken, syncToken := extractMediaItemsFromResponse(data)

	result.Items = items
	result.NextPageToken = paginationToken
	result.SyncToken = syncToken

	return result, nil
}

// extractMediaItemsFromResponse parses the protobuf response bytes and extracts media items
func extractMediaItemsFromResponse(data []byte) ([]MediaItem, string, string) {
	var items []MediaItem
	var paginationToken string
	var syncToken string

	// Parse the top-level message
	offset := 0
	resyncSkips := 0
	const maxResyncSkips = 256
	for offset < len(data) {
		fieldNum, wireType, newOffset := readTag(data, offset)
		if newOffset < 0 {
			break
		}
		offset = newOffset

		switch wireType {
		case 0: // Varint
			_, newOffset := readVarint(data, offset)
			if newOffset < 0 {
				if resyncSkips < maxResyncSkips {
					resyncSkips++
					offset++
					continue
				}
				return items, paginationToken, syncToken
			}
			resyncSkips = 0
			offset = newOffset
		case 2: // Length-delimited
			length, newOffset := readVarint(data, offset)
			if newOffset < 0 || newOffset+int(length) > len(data) {
				if resyncSkips < maxResyncSkips {
					resyncSkips++
					offset++
					continue
				}
				return items, paginationToken, syncToken
			}
			resyncSkips = 0
			fieldData := data[newOffset : newOffset+int(length)]
			offset = newOffset + int(length)

				// Field 1 contains the main response data
				if fieldNum == 1 {
					extractedItems, token, sToken := parseResponseField1(fieldData)
					items = append(items, extractedItems...)
					if token != "" {
						paginationToken = token
					}
					if sToken != "" {
						syncToken = sToken
					}
					// The media list lives under response field 1; avoid scanning other top-level
					// fields to ensure we only return items from 1.2.
					return items, paginationToken, syncToken
				}
		case 5: // 32-bit
			if offset+4 > len(data) {
				if resyncSkips < maxResyncSkips {
					resyncSkips++
					offset++
					continue
				}
				return items, paginationToken, syncToken
			}
			resyncSkips = 0
			offset += 4
		case 1: // 64-bit
			if offset+8 > len(data) {
				if resyncSkips < maxResyncSkips {
					resyncSkips++
					offset++
					continue
				}
				return items, paginationToken, syncToken
			}
			resyncSkips = 0
			offset += 8
		case 3: // Start group
			newOffset, ok := skipGroup(data, offset, fieldNum)
			if !ok {
				if resyncSkips < maxResyncSkips {
					resyncSkips++
					offset++
					continue
				}
				return items, paginationToken, syncToken
			}
			resyncSkips = 0
			offset = newOffset
		case 4: // End group (unexpected at top-level)
			return items, paginationToken, syncToken
		default:
			newOffset, ok := skipField(data, wireType, offset, fieldNum)
			if !ok {
				if resyncSkips < maxResyncSkips {
					resyncSkips++
					offset++
					continue
				}
				return items, paginationToken, syncToken
			}
			resyncSkips = 0
			offset = newOffset
		}
	}

	return items, paginationToken, syncToken
}

// parseResponseField1 parses the field1 of the response which contains media items
func parseResponseField1(data []byte) ([]MediaItem, string, string) {
	var items []MediaItem
	var paginationToken string
	var syncToken string

	offset := 0
	resyncSkips := 0
	const maxResyncSkips = 256
	for offset < len(data) {
		fieldNum, wireType, newOffset := readTag(data, offset)
		if newOffset < 0 {
			break
		}
		offset = newOffset

		switch wireType {
		case 0: // Varint
			_, newOffset := readVarint(data, offset)
			if newOffset < 0 {
				if resyncSkips < maxResyncSkips {
					resyncSkips++
					offset++
					continue
				}
				return items, paginationToken, syncToken
			}
			resyncSkips = 0
			offset = newOffset
		case 2: // Length-delimited
			length, newOffset := readVarint(data, offset)
			if newOffset < 0 || newOffset+int(length) > len(data) {
				if resyncSkips < maxResyncSkips {
					resyncSkips++
					offset++
					continue
				}
				return items, paginationToken, syncToken
			}
			resyncSkips = 0
			fieldData := data[newOffset : newOffset+int(length)]
			offset = newOffset + int(length)

			// Field 2 contains media items array (repeated field)
			if fieldNum == 2 {
				item := tryParseMediaItem(fieldData)
				if item != nil && item.MediaKey != "" {
					items = append(items, *item)
				}
			}
			// Field 1 is the pagination token (next_page_token)
			if fieldNum == 1 {
				paginationToken = string(fieldData)
			}
			// Field 6 is the sync token (sync_token) - new state anchor
			if fieldNum == 6 {
				syncToken = string(fieldData)
			}
		case 5: // 32-bit
			if offset+4 > len(data) {
				if resyncSkips < maxResyncSkips {
					resyncSkips++
					offset++
					continue
				}
				return items, paginationToken, syncToken
			}
			resyncSkips = 0
			offset += 4
		case 1: // 64-bit
			if offset+8 > len(data) {
				if resyncSkips < maxResyncSkips {
					resyncSkips++
					offset++
					continue
				}
				return items, paginationToken, syncToken
			}
			resyncSkips = 0
			offset += 8
		case 3: // Start group
			newOffset, ok := skipGroup(data, offset, fieldNum)
			if !ok {
				if resyncSkips < maxResyncSkips {
					resyncSkips++
					offset++
					continue
				}
				return items, paginationToken, syncToken
			}
			resyncSkips = 0
			offset = newOffset
		case 4: // End group
			return items, paginationToken, syncToken
		default:
			newOffset, ok := skipField(data, wireType, offset, fieldNum)
			if !ok {
				if resyncSkips < maxResyncSkips {
					resyncSkips++
					offset++
					continue
				}
				return items, paginationToken, syncToken
			}
			resyncSkips = 0
			offset = newOffset
		}
	}

	return items, paginationToken, syncToken
}

// tryParseMediaItem attempts to parse a protobuf message as a media item
func tryParseMediaItem(data []byte) *MediaItem {
	item := &MediaItem{CountsTowardsQuota: false}

	offset := 0
	for offset < len(data) {
		fieldNum, wireType, newOffset := readTag(data, offset)
		if newOffset < 0 {
			break
		}
		offset = newOffset

		switch wireType {
		case 0: // Varint
			val, newOffset := readVarint(data, offset)
			if newOffset < 0 {
				return item
			}
			offset = newOffset
			// Field 5 might be media type
			if fieldNum == 5 {
				if val == 1 {
					item.MediaType = "photo"
				} else if val == 2 {
					item.MediaType = "video"
				}
			}
		case 2: // Length-delimited
			length, newOffset := readVarint(data, offset)
			if newOffset < 0 || newOffset+int(length) > len(data) {
				return item
			}
			fieldData := data[newOffset : newOffset+int(length)]
			offset = newOffset + int(length)

			switch fieldNum {
			case 1:
				// Could be media key (string) or nested message
				if isPrintableString(fieldData) && len(fieldData) > minMediaKeyLength {
					item.MediaKey = string(fieldData)
				} else {
					nestedItem := tryParseMediaItem(fieldData)
					if nestedItem != nil && nestedItem.MediaKey != "" {
						item.MediaKey = nestedItem.MediaKey
						if nestedItem.Filename != "" && item.Filename == "" {
							item.Filename = nestedItem.Filename
						}
						if nestedItem.MediaType != "" && item.MediaType == "" {
							item.MediaType = nestedItem.MediaType
						}
						if nestedItem.DedupKey != "" && item.DedupKey == "" {
							item.DedupKey = nestedItem.DedupKey
						}
					}
				}
			case 2:
				// Field 2 is a nested message containing metadata including filename at sub-field 4
				filename, countsTowardsQuota, status, isTrash := extractField2Metadata(fieldData)
				if filename != "" {
					item.Filename = filename
				} else if isPrintableString(fieldData) {
					// Fallback: Could be filename or dedup key directly
					str := string(fieldData)
					if item.Filename == "" && strings.Contains(str, ".") {
						item.Filename = str
					} else if item.DedupKey == "" {
						item.DedupKey = str
					}
				}

				item.CountsTowardsQuota = item.CountsTowardsQuota || countsTowardsQuota
				if status > 0 {
					item.Status = status
				}
				if isTrash {
					item.IsTrash = true
				}
				if item.DedupKey == "" {
					item.DedupKey = extractDedupKeyFromField2(fieldData)
				}
			case 4:
				// Timestamp nested message
				ts := tryParseTimestamp(fieldData)
				if ts > 0 {
					item.Timestamp = ts
				}
			case 6:
				// Field 6 is often a nested message that also contains the media key at sub-field 1
				if item.MediaKey == "" {
					nestedItem := tryParseMediaItem(fieldData)
					if nestedItem != nil && nestedItem.MediaKey != "" {
						item.MediaKey = nestedItem.MediaKey
					}
				}
			case 22:
				if parseQuotaInfo(fieldData) {
					item.CountsTowardsQuota = true
				}
			}
		case 5: // 32-bit
			if offset+4 > len(data) {
				return item
			}
			offset += 4
		case 1: // 64-bit
			if offset+8 > len(data) {
				return item
			}
			offset += 8
		case 3: // Start group
			newOffset, ok := skipGroup(data, offset, fieldNum)
			if !ok {
				return item
			}
			offset = newOffset
		case 4: // End group
			return item
		default:
			newOffset, ok := skipField(data, wireType, offset, fieldNum)
			if !ok {
				return item
			}
			offset = newOffset
		}
	}

	return item
}

// parseQuotaInfo checks if field 22 indicates quota consumption
func parseQuotaInfo(data []byte) bool {
	offset := 0
	for offset < len(data) {
		fieldNum, wireType, newOffset := readTag(data, offset)
		if newOffset < 0 {
			break
		}
		offset = newOffset

		switch wireType {
		case 0: // Varint
			val, newOffset := readVarint(data, offset)
			if newOffset < 0 {
				return false
			}
			offset = newOffset
			// Field 1: 0 means no quota
			if fieldNum == 1 {
				if val == 0 {
					return false
				}
			}
		case 2: // Length-delimited
			length, newOffset := readVarint(data, offset)
			if newOffset < 0 || newOffset+int(length) > len(data) {
				return false
			}
			offset = newOffset + int(length)

			// Field 1: Message means quota consumed
			if fieldNum == 1 {
				return true
			}
		case 5: // 32-bit
			offset += 4
		case 1: // 64-bit
			offset += 8
		default:
			newOffset, ok := skipField(data, wireType, offset, fieldNum)
			if !ok {
				return false
			}
			offset = newOffset
		}
	}
	return false
}

// tryParseTimestamp attempts to parse a timestamp from a nested protobuf message
func tryParseTimestamp(data []byte) int64 {
	offset := 0
	for offset < len(data) {
		fieldNum, wireType, newOffset := readTag(data, offset)
		if newOffset < 0 {
			break
		}
		offset = newOffset

		if wireType == 0 && fieldNum == 1 {
			val, _ := readVarint(data, offset)
			return int64(val)
		}

		// Skip other fields
		switch wireType {
		case 0:
			_, offset = readVarint(data, offset)
		case 2:
			length, newOffset := readVarint(data, offset)
			if newOffset < 0 {
				return 0
			}
			offset = newOffset + int(length)
		case 5:
			offset += 4
		case 1:
			offset += 8
		default:
			return 0
		}
	}
	return 0
}

// readTag reads a protobuf tag from the data
func readTag(data []byte, offset int) (fieldNum int, wireType int, newOffset int) {
	if offset >= len(data) {
		return 0, 0, -1
	}
	tag, newOffset := readVarint(data, offset)
	if newOffset < 0 {
		return 0, 0, -1
	}
	return int(tag >> 3), int(tag & 0x7), newOffset
}

// readVarint reads a varint from the data
func readVarint(data []byte, offset int) (uint64, int) {
	var result uint64
	var shift uint
	for offset < len(data) {
		b := data[offset]
		offset++
		result |= uint64(b&0x7F) << shift
		if b < 0x80 {
			return result, offset
		}
		shift += 7
		if shift >= 64 {
			return 0, -1
		}
	}
	return 0, -1
}

// skipField skips over an unknown protobuf field's value starting at offset (immediately after the tag).
// It returns the updated offset and whether skipping was successful.
func skipField(data []byte, wireType int, offset int, fieldNum int) (int, bool) {
	switch wireType {
	case 0: // Varint
		_, newOffset := readVarint(data, offset)
		if newOffset < 0 {
			return offset, false
		}
		return newOffset, true
	case 1: // 64-bit
		if offset+8 > len(data) {
			return offset, false
		}
		return offset + 8, true
	case 2: // Length-delimited
		length, newOffset := readVarint(data, offset)
		if newOffset < 0 || newOffset+int(length) > len(data) {
			return offset, false
		}
		return newOffset + int(length), true
	case 3: // Start group (deprecated but still possible)
		return skipGroup(data, offset, fieldNum)
	case 4: // End group
		// Caller should handle end-group; don't advance here.
		return offset, true
	case 5: // 32-bit
		if offset+4 > len(data) {
			return offset, false
		}
		return offset + 4, true
	default:
		return offset, false
	}
}

// skipGroup skips a protobuf group starting at offset (immediately after the start-group tag).
// It returns the offset after the matching end-group tag.
func skipGroup(data []byte, offset int, groupFieldNum int) (int, bool) {
	for offset < len(data) {
		fieldNum, wireType, newOffset := readTag(data, offset)
		if newOffset < 0 {
			return offset, false
		}
		offset = newOffset

		// End-group tag matching our group's field number.
		if wireType == 4 && fieldNum == groupFieldNum {
			return offset, true
		}

		var ok bool
		offset, ok = skipField(data, wireType, offset, fieldNum)
		if !ok {
			return offset, false
		}
	}
	return offset, false
}

// isPrintableString checks if the byte slice contains valid printable characters
func isPrintableString(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	// Check UTF-8 validity and that all characters are printable
	// Use DecodeRune to iterate without creating a string
	for i := 0; i < len(data); {
		r, size := utf8.DecodeRune(data[i:])
		if r == utf8.RuneError && size == 1 {
			// Invalid UTF-8
			return false
		}
		// Check for control characters (except whitespace)
		if r < 32 && r != '\t' && r != '\n' && r != '\r' {
			return false
		}
		i += size
	}
	return true
}

// GetAlbumList retrieves a list of albums from Google Photos
// This uses a specific protobuf format for requesting album lists
// pageToken should be passed from previous responses for proper pagination
func (a *Api) GetAlbumList(pageToken string) (*AlbumListResult, error) {
	// Build the request using the exact protobuf structure
	requestData := buildAlbumListRequest(pageToken)

	// Get the bearer token
	bearerToken, err := a.BearerToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get bearer token: %w", err)
	}

	// Prepare headers
	headers := map[string]string{
		"accept-encoding":          "gzip",
		"Accept-Language":          a.language,
		"Content-Type":             "application/x-protobuf",
		"User-Agent":               a.userAgent,
		"Authorization":            "Bearer " + bearerToken,
		"x-goog-ext-173412678-bin": "CgcIAhClARgC",
		"x-goog-ext-174067345-bin": "CgIIAg==",
	}

	// Create the request
	req, err := http.NewRequest(
		"POST",
		"https://photosdata-pa.googleapis.com/6439526531001121323/18047484249733410717",
		bytes.NewReader(requestData),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Make the request
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check for errors
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Handle gzip response if needed
	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		reader, err = gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer reader.(*gzip.Reader).Close()
	}

	// Read the response body
	bodyBytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse the response to extract albums
	result, err := parseAlbumListResponse(bodyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

// buildAlbumListRequest creates the protobuf request for fetching album list
// According to the provided format, only field 1.4 (pageToken) changes between requests
func buildAlbumListRequest(pageToken string) []byte {
	var buf bytes.Buffer

	// Build field 1 (main request data)
	field1 := buildAlbumListRequestField1(pageToken)
	writeProtobufField(&buf, 1, field1)

	// Build field 2 (additional options)
	field2 := buildAlbumListRequestField2()
	writeProtobufField(&buf, 2, field2)

	return buf.Bytes()
}

// buildAlbumListRequestField1 builds the complex nested field 1 structure
func buildAlbumListRequestField1(pageToken string) []byte {
	var buf bytes.Buffer

	// field 1.1 - nested message with media/album metadata options
	field1_1 := buildAlbumListField1_1()
	writeProtobufField(&buf, 1, field1_1)

	// field 1.2 - nested message with various options
	field1_2 := buildAlbumListField1_2()
	writeProtobufField(&buf, 2, field1_2)

	// field 1.3 - nested message with collection options
	field1_3 := buildAlbumListField1_3()
	writeProtobufField(&buf, 3, field1_3)

	// field 1.4 - pagination token (string) - THE ONLY FIELD THAT CHANGES
	if pageToken != "" {
		writeProtobufString(&buf, 4, pageToken)
	}

	// field 1.7 - type (varint = 2)
	writeProtobufVarint(&buf, 7, 2)

	// field 1.9 - nested message
	field1_9 := buildAlbumListField1_9()
	writeProtobufField(&buf, 9, field1_9)

	// field 1.11 - repeated ints [1, 2, 6]
	writeProtobufVarint(&buf, 11, 1)
	writeProtobufVarint(&buf, 11, 2)
	writeProtobufVarint(&buf, 11, 6)

	// field 1.12 - nested message
	field1_12 := buildAlbumListField1_12()
	writeProtobufField(&buf, 12, field1_12)

	// field 1.13 - empty string
	writeProtobufString(&buf, 13, "")

	// field 1.15 - nested message
	field1_15 := buildAlbumListField1_15()
	writeProtobufField(&buf, 15, field1_15)

	// field 1.18 - nested message with specific ID
	field1_18 := buildAlbumListField1_18()
	writeProtobufField(&buf, 18, field1_18)

	// field 1.19 - nested message
	field1_19 := buildAlbumListField1_19()
	writeProtobufField(&buf, 19, field1_19)

	// field 1.20 - nested message
	field1_20 := buildAlbumListField1_20()
	writeProtobufField(&buf, 20, field1_20)

	// field 1.21 - nested message
	field1_21 := buildAlbumListField1_21()
	writeProtobufField(&buf, 21, field1_21)

	// field 1.22 - nested message
	field1_22 := buildAlbumListField1_22()
	writeProtobufField(&buf, 22, field1_22)

	// field 1.25 - nested message
	field1_25 := buildAlbumListField1_25()
	writeProtobufField(&buf, 25, field1_25)

	// field 1.26 - empty string
	writeProtobufString(&buf, 26, "")

	return buf.Bytes()
}

// buildAlbumListField1_1 builds field 1.1 - media/album metadata options
func buildAlbumListField1_1() []byte {
	var buf bytes.Buffer

	// field 1.1.1 - nested message with all metadata fields
	var field1_1_1 bytes.Buffer

	// Empty fields: 1, 3, 4, 6, 15, 16, 17, 19, 20, 25, 31, 32, 34, 36, 37, 38, 39, 40, 41, 42
	emptyFields := []int{1, 3, 4, 6, 15, 16, 17, 19, 20, 25, 31, 32, 34, 36, 37, 38, 39, 40, 41, 42}
	for _, f := range emptyFields {
		writeProtobufString(&field1_1_1, f, "")
	}

	// field 1.1.1.5 - nested message
	var field5 bytes.Buffer
	for _, f := range []int{1, 2, 3, 4, 5, 7} {
		writeProtobufString(&field5, f, "")
	}
	writeProtobufField(&field1_1_1, 5, field5.Bytes())

	// field 1.1.1.7 - nested message
	var field7 bytes.Buffer
	writeProtobufString(&field7, 2, "")
	writeProtobufField(&field1_1_1, 7, field7.Bytes())

	// field 1.1.1.21 - nested message
	var field21 bytes.Buffer
	var field21_5 bytes.Buffer
	writeProtobufString(&field21_5, 3, "")
	writeProtobufField(&field21, 5, field21_5.Bytes())
	writeProtobufString(&field21, 6, "")
	var field21_7 bytes.Buffer
	writeProtobufVarint(&field21_7, 2, 0)
	writeProtobufVarint(&field21_7, 3, 1)
	writeProtobufField(&field21, 7, field21_7.Bytes())
	writeProtobufField(&field1_1_1, 21, field21.Bytes())

	// field 1.1.1.30 - nested message
	var field30 bytes.Buffer
	writeProtobufString(&field30, 2, "")
	writeProtobufField(&field1_1_1, 30, field30.Bytes())

	// field 1.1.1.33 - nested message
	var field33 bytes.Buffer
	writeProtobufString(&field33, 1, "")
	writeProtobufField(&field1_1_1, 33, field33.Bytes())

	writeProtobufField(&buf, 1, field1_1_1.Bytes())
	return buf.Bytes()
}

// buildAlbumListField1_2 builds field 1.2 - complex nested options
func buildAlbumListField1_2() []byte {
	var buf bytes.Buffer

	// field 1.2.1 - nested message
	var field1_2_1 bytes.Buffer
	for _, f := range []int{2, 3, 4, 5, 7, 8, 10, 12, 18} {
		writeProtobufString(&field1_2_1, f, "")
	}

	// field 1.2.1.6 - nested
	var field1_2_1_6 bytes.Buffer
	for _, f := range []int{1, 2, 3, 4, 5, 7} {
		writeProtobufString(&field1_2_1_6, f, "")
	}
	writeProtobufField(&field1_2_1, 6, field1_2_1_6.Bytes())

	// field 1.2.1.13 - nested
	var field1_2_1_13 bytes.Buffer
	writeProtobufString(&field1_2_1_13, 2, "")
	writeProtobufString(&field1_2_1_13, 3, "")
	writeProtobufField(&field1_2_1, 13, field1_2_1_13.Bytes())

	// field 1.2.1.15 - nested
	var field1_2_1_15 bytes.Buffer
	writeProtobufString(&field1_2_1_15, 1, "")
	writeProtobufField(&field1_2_1, 15, field1_2_1_15.Bytes())

	writeProtobufField(&buf, 1, field1_2_1.Bytes())

	// field 1.2.4 - nested
	var field1_2_4 bytes.Buffer
	var field1_2_4_1 bytes.Buffer
	writeProtobufString(&field1_2_4_1, 1, "")
	writeProtobufField(&field1_2_4, 1, field1_2_4_1.Bytes())
	writeProtobufField(&buf, 4, field1_2_4.Bytes())

	// field 1.2.9 - empty
	writeProtobufString(&buf, 9, "")

	// field 1.2.11 - nested
	var field1_2_11 bytes.Buffer
	var field1_2_11_1 bytes.Buffer
	for _, f := range []int{1, 4, 5, 6, 9} {
		writeProtobufString(&field1_2_11_1, f, "")
	}
	writeProtobufField(&field1_2_11, 1, field1_2_11_1.Bytes())
	writeProtobufField(&buf, 11, field1_2_11.Bytes())

	// field 1.2.14 - complex nested
	var field1_2_14 bytes.Buffer
	var field1_2_14_1 bytes.Buffer

	// field 1.2.14.1.1
	var field1_2_14_1_1 bytes.Buffer
	writeProtobufString(&field1_2_14_1_1, 1, "")

	// field 1.2.14.1.1.2
	var field1_2_14_1_1_2 bytes.Buffer
	var field1_2_14_1_1_2_2 bytes.Buffer
	var field1_2_14_1_1_2_2_1 bytes.Buffer
	writeProtobufString(&field1_2_14_1_1_2_2_1, 1, "")
	writeProtobufField(&field1_2_14_1_1_2_2, 1, field1_2_14_1_1_2_2_1.Bytes())
	writeProtobufString(&field1_2_14_1_1_2_2, 3, "")
	writeProtobufField(&field1_2_14_1_1_2, 2, field1_2_14_1_1_2_2.Bytes())
	writeProtobufField(&field1_2_14_1_1, 2, field1_2_14_1_1_2.Bytes())

	// field 1.2.14.1.1.3
	var field1_2_14_1_1_3 bytes.Buffer

	// field 1.2.14.1.1.3.4
	var field1_2_14_1_1_3_4 bytes.Buffer
	var field1_2_14_1_1_3_4_1 bytes.Buffer
	writeProtobufString(&field1_2_14_1_1_3_4_1, 1, "")
	writeProtobufField(&field1_2_14_1_1_3_4, 1, field1_2_14_1_1_3_4_1.Bytes())
	writeProtobufString(&field1_2_14_1_1_3_4, 3, "")
	writeProtobufField(&field1_2_14_1_1_3, 4, field1_2_14_1_1_3_4.Bytes())

	// field 1.2.14.1.1.3.5
	var field1_2_14_1_1_3_5 bytes.Buffer
	var field1_2_14_1_1_3_5_1 bytes.Buffer
	writeProtobufString(&field1_2_14_1_1_3_5_1, 1, "")
	writeProtobufField(&field1_2_14_1_1_3_5, 1, field1_2_14_1_1_3_5_1.Bytes())
	writeProtobufString(&field1_2_14_1_1_3_5, 3, "")
	writeProtobufField(&field1_2_14_1_1_3, 5, field1_2_14_1_1_3_5.Bytes())

	writeProtobufField(&field1_2_14_1_1, 3, field1_2_14_1_1_3.Bytes())
	writeProtobufField(&field1_2_14_1, 1, field1_2_14_1_1.Bytes())
	writeProtobufString(&field1_2_14_1, 2, "")
	writeProtobufField(&field1_2_14, 1, field1_2_14_1.Bytes())
	writeProtobufField(&buf, 14, field1_2_14.Bytes())

	// field 1.2.17 - empty
	writeProtobufString(&buf, 17, "")

	// field 1.2.18 - nested
	var field1_2_18 bytes.Buffer
	writeProtobufString(&field1_2_18, 1, "")
	var field1_2_18_2 bytes.Buffer
	writeProtobufString(&field1_2_18_2, 1, "")
	writeProtobufField(&field1_2_18, 2, field1_2_18_2.Bytes())
	writeProtobufField(&buf, 18, field1_2_18.Bytes())

	// field 1.2.20 - nested
	var field1_2_20 bytes.Buffer
	var field1_2_20_2 bytes.Buffer
	writeProtobufString(&field1_2_20_2, 1, "")
	writeProtobufString(&field1_2_20_2, 2, "")
	writeProtobufField(&field1_2_20, 2, field1_2_20_2.Bytes())
	writeProtobufField(&buf, 20, field1_2_20.Bytes())

	// field 1.2.22 and 1.2.23 - empty
	writeProtobufString(&buf, 22, "")
	writeProtobufString(&buf, 23, "")

	return buf.Bytes()
}

// buildAlbumListField1_3 builds field 1.3 - collection options
func buildAlbumListField1_3() []byte {
	var buf bytes.Buffer

	// field 1.3.2 - empty
	writeProtobufString(&buf, 2, "")

	// field 1.3.3 - nested with many empty fields
	var field1_3_3 bytes.Buffer
	emptyFields := []int{2, 3, 7, 8, 16, 18, 19, 20, 21, 22, 23, 29, 30, 31, 32, 34, 37, 38, 39, 41, 47}
	for _, f := range emptyFields {
		writeProtobufString(&field1_3_3, f, "")
	}

	// field 1.3.3.14
	var field1_3_3_14 bytes.Buffer
	writeProtobufString(&field1_3_3_14, 1, "")
	writeProtobufField(&field1_3_3, 14, field1_3_3_14.Bytes())

	// field 1.3.3.17
	var field1_3_3_17 bytes.Buffer
	writeProtobufString(&field1_3_3_17, 2, "")
	writeProtobufField(&field1_3_3, 17, field1_3_3_17.Bytes())

	// field 1.3.3.27
	var field1_3_3_27 bytes.Buffer
	writeProtobufString(&field1_3_3_27, 1, "")
	var field1_3_3_27_2 bytes.Buffer
	writeProtobufString(&field1_3_3_27_2, 1, "")
	writeProtobufField(&field1_3_3_27, 2, field1_3_3_27_2.Bytes())
	writeProtobufField(&field1_3_3, 27, field1_3_3_27.Bytes())

	// field 1.3.3.45
	var field1_3_3_45 bytes.Buffer
	var field1_3_3_45_1 bytes.Buffer
	writeProtobufString(&field1_3_3_45_1, 1, "")
	writeProtobufField(&field1_3_3_45, 1, field1_3_3_45_1.Bytes())
	writeProtobufField(&field1_3_3, 45, field1_3_3_45.Bytes())

	// field 1.3.3.46
	var field1_3_3_46 bytes.Buffer
	writeProtobufString(&field1_3_3_46, 1, "")
	var field1_3_3_46_2 bytes.Buffer
	var field1_3_3_46_2_1 bytes.Buffer
	writeProtobufString(&field1_3_3_46_2_1, 1, "")
	writeProtobufField(&field1_3_3_46_2, 1, field1_3_3_46_2_1.Bytes())
	writeProtobufField(&field1_3_3_46, 2, field1_3_3_46_2.Bytes())
	writeProtobufString(&field1_3_3_46, 3, "")
	writeProtobufField(&field1_3_3, 46, field1_3_3_46.Bytes())

	writeProtobufField(&buf, 3, field1_3_3.Bytes())

	// field 1.3.4 - nested
	var field1_3_4 bytes.Buffer
	writeProtobufString(&field1_3_4, 2, "")
	var field1_3_4_3 bytes.Buffer
	writeProtobufString(&field1_3_4_3, 1, "")
	writeProtobufField(&field1_3_4, 3, field1_3_4_3.Bytes())
	writeProtobufString(&field1_3_4, 4, "")
	var field1_3_4_5 bytes.Buffer
	writeProtobufString(&field1_3_4_5, 1, "")
	writeProtobufField(&field1_3_4, 5, field1_3_4_5.Bytes())
	writeProtobufField(&buf, 4, field1_3_4.Bytes())

	// field 1.3.7 - empty
	writeProtobufString(&buf, 7, "")

	// field 1.3.8 - nested
	var field1_3_8 bytes.Buffer
	var field1_3_8_2 bytes.Buffer
	writeProtobufVarint(&field1_3_8_2, 1, 1)
	writeProtobufVarint(&field1_3_8_2, 2, 1)
	writeProtobufField(&field1_3_8, 2, field1_3_8_2.Bytes())
	writeProtobufField(&buf, 8, field1_3_8.Bytes())

	// fields 1.3.12, 1.3.13, 1.3.15, 1.3.18, 1.3.20, 1.3.22, 1.3.24, 1.3.25
	for _, f := range []int{12, 13, 15, 18, 20, 22, 24, 25} {
		writeProtobufString(&buf, f, "")
	}

	// field 1.3.14 - complex nested
	field1_3_14 := buildAlbumListField1_3_14()
	writeProtobufField(&buf, 14, field1_3_14)

	// field 1.3.16 - nested
	var field1_3_16 bytes.Buffer
	writeProtobufString(&field1_3_16, 1, "")
	writeProtobufField(&buf, 16, field1_3_16.Bytes())

	// field 1.3.19 - nested
	field1_3_19 := buildAlbumListField1_3_19()
	writeProtobufField(&buf, 19, field1_3_19)

	return buf.Bytes()
}

// buildAlbumListField1_3_14 builds the complex nested field 1.3.14
func buildAlbumListField1_3_14() []byte {
	var buf bytes.Buffer

	// field 1.3.14.1 - empty
	writeProtobufString(&buf, 1, "")

	// field 1.3.14.2 - nested
	var field1_3_14_2 bytes.Buffer
	writeProtobufString(&field1_3_14_2, 1, "")
	var field1_3_14_2_2 bytes.Buffer
	writeProtobufString(&field1_3_14_2_2, 1, "")
	writeProtobufField(&field1_3_14_2, 2, field1_3_14_2_2.Bytes())
	writeProtobufString(&field1_3_14_2, 3, "")
	var field1_3_14_2_4 bytes.Buffer
	writeProtobufString(&field1_3_14_2_4, 1, "")
	writeProtobufField(&field1_3_14_2, 4, field1_3_14_2_4.Bytes())
	writeProtobufField(&buf, 2, field1_3_14_2.Bytes())

	// field 1.3.14.3 - nested
	var field1_3_14_3 bytes.Buffer
	writeProtobufString(&field1_3_14_3, 1, "")
	var field1_3_14_3_2 bytes.Buffer
	writeProtobufString(&field1_3_14_3_2, 1, "")
	writeProtobufField(&field1_3_14_3, 2, field1_3_14_3_2.Bytes())
	writeProtobufString(&field1_3_14_3, 3, "")
	writeProtobufString(&field1_3_14_3, 4, "")
	writeProtobufField(&buf, 3, field1_3_14_3.Bytes())

	return buf.Bytes()
}

// buildAlbumListField1_3_19 builds field 1.3.19
func buildAlbumListField1_3_19() []byte {
	var buf bytes.Buffer

	// field 1.3.19.4 - nested
	var field1_3_19_4 bytes.Buffer
	writeProtobufString(&field1_3_19_4, 2, "")
	writeProtobufField(&buf, 4, field1_3_19_4.Bytes())

	// field 1.3.19.6 - nested
	var field1_3_19_6 bytes.Buffer
	writeProtobufString(&field1_3_19_6, 2, "")
	writeProtobufString(&field1_3_19_6, 3, "")
	writeProtobufField(&buf, 6, field1_3_19_6.Bytes())

	// field 1.3.19.7 - nested
	var field1_3_19_7 bytes.Buffer
	writeProtobufString(&field1_3_19_7, 2, "")
	writeProtobufString(&field1_3_19_7, 3, "")
	writeProtobufField(&buf, 7, field1_3_19_7.Bytes())

	// field 1.3.19.8 - empty
	writeProtobufString(&buf, 8, "")

	return buf.Bytes()
}

// buildAlbumListField1_9 builds field 1.9
func buildAlbumListField1_9() []byte {
	var buf bytes.Buffer

	// field 1.9.1 - nested
	var field1_9_1 bytes.Buffer
	var field1_9_1_2 bytes.Buffer
	writeProtobufString(&field1_9_1_2, 1, "")
	writeProtobufString(&field1_9_1_2, 2, "")
	writeProtobufField(&field1_9_1, 2, field1_9_1_2.Bytes())
	writeProtobufField(&buf, 1, field1_9_1.Bytes())

	// field 1.9.2 - nested
	var field1_9_2 bytes.Buffer
	var field1_9_2_3 bytes.Buffer
	writeProtobufVarint(&field1_9_2_3, 2, 1)
	writeProtobufField(&field1_9_2, 3, field1_9_2_3.Bytes())
	writeProtobufField(&buf, 2, field1_9_2.Bytes())

	// field 1.9.3 - nested
	var field1_9_3 bytes.Buffer
	writeProtobufString(&field1_9_3, 2, "")
	writeProtobufField(&buf, 3, field1_9_3.Bytes())

	// field 1.9.4 - empty
	writeProtobufString(&buf, 4, "")

	// field 1.9.7 - nested
	var field1_9_7 bytes.Buffer
	writeProtobufString(&field1_9_7, 1, "")
	writeProtobufField(&buf, 7, field1_9_7.Bytes())

	// field 1.9.8 - nested
	var field1_9_8 bytes.Buffer
	writeProtobufVarint(&field1_9_8, 1, 2)
	// repeated field
	for _, v := range []int64{1, 2, 3, 5, 6} {
		writeProtobufVarint(&field1_9_8, 2, v)
	}
	writeProtobufField(&buf, 8, field1_9_8.Bytes())

	// field 1.9.9 - empty
	writeProtobufString(&buf, 9, "")

	return buf.Bytes()
}

// buildAlbumListField1_12 builds field 1.12
func buildAlbumListField1_12() []byte {
	var buf bytes.Buffer

	// field 1.12.2 - nested
	var field1_12_2 bytes.Buffer
	writeProtobufString(&field1_12_2, 1, "")
	writeProtobufString(&field1_12_2, 2, "")
	writeProtobufField(&buf, 2, field1_12_2.Bytes())

	// field 1.12.3 - nested
	var field1_12_3 bytes.Buffer
	writeProtobufString(&field1_12_3, 1, "")
	writeProtobufField(&buf, 3, field1_12_3.Bytes())

	// field 1.12.4 - empty
	writeProtobufString(&buf, 4, "")

	return buf.Bytes()
}

// buildAlbumListField1_15 builds field 1.15
func buildAlbumListField1_15() []byte {
	var buf bytes.Buffer

	// field 1.15.3 - nested
	var field1_15_3 bytes.Buffer
	writeProtobufVarint(&field1_15_3, 1, 1)
	writeProtobufField(&buf, 3, field1_15_3.Bytes())

	return buf.Bytes()
}

// buildAlbumListField1_18 builds field 1.18
func buildAlbumListField1_18() []byte {
	var buf bytes.Buffer

	// field 1.18 contains a specific ID (169945741) as the field number
	var field169945741 bytes.Buffer
	var field169945741_1 bytes.Buffer
	var field169945741_1_1 bytes.Buffer

	// repeated field
	for _, v := range []int64{2, 1, 6, 8, 10, 15, 18, 13, 17, 19, 14, 20} {
		writeProtobufVarint(&field169945741_1_1, 4, v)
	}
	writeProtobufVarint(&field169945741_1_1, 5, 6)
	writeProtobufVarint(&field169945741_1_1, 6, 2)
	writeProtobufVarint(&field169945741_1_1, 7, 1)
	writeProtobufVarint(&field169945741_1_1, 8, 2)
	writeProtobufVarint(&field169945741_1_1, 11, 3)
	writeProtobufVarint(&field169945741_1_1, 12, 1)
	writeProtobufVarint(&field169945741_1_1, 13, 3)
	writeProtobufVarint(&field169945741_1_1, 15, 1)
	writeProtobufVarint(&field169945741_1_1, 16, 1)
	writeProtobufVarint(&field169945741_1_1, 17, 1)
	writeProtobufVarint(&field169945741_1_1, 18, 2)

	writeProtobufField(&field169945741_1, 1, field169945741_1_1.Bytes())
	writeProtobufField(&field169945741, 1, field169945741_1.Bytes())
	writeProtobufField(&buf, 169945741, field169945741.Bytes())

	return buf.Bytes()
}

// buildAlbumListField1_19 builds field 1.19
func buildAlbumListField1_19() []byte {
	var buf bytes.Buffer

	// field 1.19.1 - nested
	var field1_19_1 bytes.Buffer
	writeProtobufString(&field1_19_1, 1, "")
	writeProtobufString(&field1_19_1, 2, "")
	writeProtobufField(&buf, 1, field1_19_1.Bytes())

	// field 1.19.2 - nested with repeated
	var field1_19_2 bytes.Buffer
	for _, v := range []int64{1, 2, 4, 6, 5, 7} {
		writeProtobufVarint(&field1_19_2, 1, v)
	}
	writeProtobufField(&buf, 2, field1_19_2.Bytes())

	// field 1.19.3 - nested
	var field1_19_3 bytes.Buffer
	writeProtobufString(&field1_19_3, 1, "")
	writeProtobufString(&field1_19_3, 2, "")
	writeProtobufField(&buf, 3, field1_19_3.Bytes())

	// field 1.19.5 - nested
	var field1_19_5 bytes.Buffer
	writeProtobufString(&field1_19_5, 1, "")
	writeProtobufString(&field1_19_5, 2, "")
	writeProtobufField(&buf, 5, field1_19_5.Bytes())

	// field 1.19.6 - nested
	var field1_19_6 bytes.Buffer
	writeProtobufString(&field1_19_6, 1, "")
	writeProtobufField(&buf, 6, field1_19_6.Bytes())

	// field 1.19.7 - nested
	var field1_19_7 bytes.Buffer
	writeProtobufString(&field1_19_7, 1, "")
	writeProtobufString(&field1_19_7, 2, "")
	writeProtobufField(&buf, 7, field1_19_7.Bytes())

	// field 1.19.8 - nested
	var field1_19_8 bytes.Buffer
	writeProtobufString(&field1_19_8, 1, "")
	writeProtobufField(&buf, 8, field1_19_8.Bytes())

	return buf.Bytes()
}

// buildAlbumListField1_20 builds field 1.20
func buildAlbumListField1_20() []byte {
	var buf bytes.Buffer

	// field 1.20.1
	writeProtobufVarint(&buf, 1, 1)

	// field 1.20.3 - nested
	var field1_20_3 bytes.Buffer
	writeProtobufString(&field1_20_3, 1, "type.googleapis.com/photos.printing.client.PrintingPromotionSyncOptions")

	var field1_20_3_2 bytes.Buffer
	var field1_20_3_2_1 bytes.Buffer

	// repeated field
	for _, v := range []int64{2, 1, 6, 8, 10, 15, 18, 13, 17, 19, 14, 20} {
		writeProtobufVarint(&field1_20_3_2_1, 4, v)
	}
	writeProtobufVarint(&field1_20_3_2_1, 5, 6)
	writeProtobufVarint(&field1_20_3_2_1, 6, 2)
	writeProtobufVarint(&field1_20_3_2_1, 7, 1)
	writeProtobufVarint(&field1_20_3_2_1, 8, 2)
	writeProtobufVarint(&field1_20_3_2_1, 11, 3)
	writeProtobufVarint(&field1_20_3_2_1, 12, 1)
	writeProtobufVarint(&field1_20_3_2_1, 13, 3)
	writeProtobufVarint(&field1_20_3_2_1, 15, 1)
	writeProtobufVarint(&field1_20_3_2_1, 16, 1)
	writeProtobufVarint(&field1_20_3_2_1, 17, 1)
	writeProtobufVarint(&field1_20_3_2_1, 18, 2)

	writeProtobufField(&field1_20_3_2, 1, field1_20_3_2_1.Bytes())
	writeProtobufField(&field1_20_3, 2, field1_20_3_2.Bytes())
	writeProtobufField(&buf, 3, field1_20_3.Bytes())

	return buf.Bytes()
}

// buildAlbumListField1_21 builds field 1.21
func buildAlbumListField1_21() []byte {
	var buf bytes.Buffer

	// field 1.21.2 - nested
	var field1_21_2 bytes.Buffer
	writeProtobufString(&field1_21_2, 2, "")
	writeProtobufString(&field1_21_2, 4, "")
	writeProtobufString(&field1_21_2, 5, "")
	writeProtobufField(&buf, 2, field1_21_2.Bytes())

	// field 1.21.3 - nested
	var field1_21_3 bytes.Buffer
	var field1_21_3_2 bytes.Buffer
	writeProtobufVarint(&field1_21_3_2, 1, 1)
	writeProtobufField(&field1_21_3, 2, field1_21_3_2.Bytes())

	var field1_21_3_4 bytes.Buffer
	writeProtobufString(&field1_21_3_4, 2, "")
	var field1_21_3_4_7 bytes.Buffer
	writeProtobufVarint(&field1_21_3_4_7, 2, 0)
	writeProtobufField(&field1_21_3_4, 7, field1_21_3_4_7.Bytes())
	writeProtobufField(&field1_21_3, 4, field1_21_3_4.Bytes())

	writeProtobufString(&field1_21_3, 8, "")
	writeProtobufField(&buf, 3, field1_21_3.Bytes())

	// field 1.21.5 - nested
	var field1_21_5 bytes.Buffer
	writeProtobufString(&field1_21_5, 1, "")
	writeProtobufField(&buf, 5, field1_21_5.Bytes())

	// field 1.21.6 - nested
	var field1_21_6 bytes.Buffer
	writeProtobufString(&field1_21_6, 1, "")
	var field1_21_6_2 bytes.Buffer
	writeProtobufString(&field1_21_6_2, 1, "")
	writeProtobufField(&field1_21_6, 2, field1_21_6_2.Bytes())
	writeProtobufField(&buf, 6, field1_21_6.Bytes())

	// field 1.21.7 - nested
	var field1_21_7 bytes.Buffer
	writeProtobufVarint(&field1_21_7, 1, 2)
	// repeated field with many values
	for _, v := range []int64{1, 7, 8, 9, 10, 13, 14, 15, 17, 19, 20, 22, 23, 45, 46, 47, 48, 49, 58, 6, 24, 50, 54, 55, 59, 62, 63, 64, 65, 56, 57, 60, 69} {
		writeProtobufVarint(&field1_21_7, 2, v)
	}
	writeProtobufVarint(&field1_21_7, 3, 1)
	writeProtobufField(&buf, 7, field1_21_7.Bytes())

	// field 1.21.8 - complex nested
	var field1_21_8 bytes.Buffer

	// field 1.21.8.3
	var field1_21_8_3 bytes.Buffer
	var field1_21_8_3_1 bytes.Buffer
	var field1_21_8_3_1_1 bytes.Buffer

	var field1_21_8_3_1_1_2 bytes.Buffer
	writeProtobufVarint(&field1_21_8_3_1_1_2, 1, 1)
	writeProtobufField(&field1_21_8_3_1_1, 2, field1_21_8_3_1_1_2.Bytes())

	var field1_21_8_3_1_1_4 bytes.Buffer
	writeProtobufString(&field1_21_8_3_1_1_4, 2, "")
	var field1_21_8_3_1_1_4_7 bytes.Buffer
	writeProtobufVarint(&field1_21_8_3_1_1_4_7, 2, 0)
	writeProtobufField(&field1_21_8_3_1_1_4, 7, field1_21_8_3_1_1_4_7.Bytes())
	writeProtobufField(&field1_21_8_3_1_1, 4, field1_21_8_3_1_1_4.Bytes())

	writeProtobufString(&field1_21_8_3_1_1, 8, "")
	writeProtobufField(&field1_21_8_3_1, 1, field1_21_8_3_1_1.Bytes())
	writeProtobufField(&field1_21_8_3, 1, field1_21_8_3_1.Bytes())
	writeProtobufString(&field1_21_8_3, 3, "")
	writeProtobufField(&field1_21_8, 3, field1_21_8_3.Bytes())

	// field 1.21.8.4
	var field1_21_8_4 bytes.Buffer
	writeProtobufString(&field1_21_8_4, 1, "")
	writeProtobufField(&field1_21_8, 4, field1_21_8_4.Bytes())

	// field 1.21.8.5
	var field1_21_8_5 bytes.Buffer
	var field1_21_8_5_1 bytes.Buffer
	var field1_21_8_5_1_2 bytes.Buffer
	writeProtobufVarint(&field1_21_8_5_1_2, 1, 1)
	writeProtobufField(&field1_21_8_5_1, 2, field1_21_8_5_1_2.Bytes())
	var field1_21_8_5_1_4 bytes.Buffer
	writeProtobufString(&field1_21_8_5_1_4, 2, "")
	var field1_21_8_5_1_4_7 bytes.Buffer
	writeProtobufVarint(&field1_21_8_5_1_4_7, 2, 0)
	writeProtobufField(&field1_21_8_5_1_4, 7, field1_21_8_5_1_4_7.Bytes())
	writeProtobufField(&field1_21_8_5_1, 4, field1_21_8_5_1_4.Bytes())
	writeProtobufString(&field1_21_8_5_1, 8, "")
	writeProtobufField(&field1_21_8_5, 1, field1_21_8_5_1.Bytes())
	writeProtobufField(&field1_21_8, 5, field1_21_8_5.Bytes())

	// field 1.21.8.6
	var field1_21_8_6 bytes.Buffer

	// field 1.21.8.6.1
	var field1_21_8_6_1 bytes.Buffer
	var field1_21_8_6_1_1 bytes.Buffer
	var field1_21_8_6_1_1_2 bytes.Buffer
	writeProtobufVarint(&field1_21_8_6_1_1_2, 1, 1)
	writeProtobufField(&field1_21_8_6_1_1, 2, field1_21_8_6_1_1_2.Bytes())
	var field1_21_8_6_1_1_4 bytes.Buffer
	writeProtobufString(&field1_21_8_6_1_1_4, 2, "")
	var field1_21_8_6_1_1_4_7 bytes.Buffer
	writeProtobufVarint(&field1_21_8_6_1_1_4_7, 2, 0)
	writeProtobufField(&field1_21_8_6_1_1_4, 7, field1_21_8_6_1_1_4_7.Bytes())
	writeProtobufField(&field1_21_8_6_1_1, 4, field1_21_8_6_1_1_4.Bytes())
	writeProtobufString(&field1_21_8_6_1_1, 8, "")
	writeProtobufField(&field1_21_8_6_1, 1, field1_21_8_6_1_1.Bytes())
	writeProtobufField(&field1_21_8_6, 1, field1_21_8_6_1.Bytes())

	// field 1.21.8.6.2
	var field1_21_8_6_2 bytes.Buffer
	var field1_21_8_6_2_1 bytes.Buffer
	var field1_21_8_6_2_1_2 bytes.Buffer
	writeProtobufVarint(&field1_21_8_6_2_1_2, 1, 1)
	writeProtobufField(&field1_21_8_6_2_1, 2, field1_21_8_6_2_1_2.Bytes())
	var field1_21_8_6_2_1_4 bytes.Buffer
	writeProtobufString(&field1_21_8_6_2_1_4, 2, "")
	var field1_21_8_6_2_1_4_7 bytes.Buffer
	writeProtobufVarint(&field1_21_8_6_2_1_4_7, 2, 0)
	writeProtobufField(&field1_21_8_6_2_1_4, 7, field1_21_8_6_2_1_4_7.Bytes())
	writeProtobufField(&field1_21_8_6_2_1, 4, field1_21_8_6_2_1_4.Bytes())
	writeProtobufString(&field1_21_8_6_2_1, 8, "")
	writeProtobufField(&field1_21_8_6_2, 1, field1_21_8_6_2_1.Bytes())
	writeProtobufField(&field1_21_8_6, 2, field1_21_8_6_2.Bytes())

	writeProtobufField(&field1_21_8, 6, field1_21_8_6.Bytes())
	writeProtobufField(&buf, 8, field1_21_8.Bytes())

	// field 1.21.9
	var field1_21_9 bytes.Buffer
	writeProtobufString(&field1_21_9, 1, "")
	writeProtobufField(&buf, 9, field1_21_9.Bytes())

	// field 1.21.10 - nested
	var field1_21_10 bytes.Buffer
	var field1_21_10_1 bytes.Buffer
	writeProtobufString(&field1_21_10_1, 1, "")
	writeProtobufField(&field1_21_10, 1, field1_21_10_1.Bytes())
	for _, f := range []int{3, 5, 7, 9, 10} {
		writeProtobufString(&field1_21_10, f, "")
	}
	var field1_21_10_6 bytes.Buffer
	writeProtobufString(&field1_21_10_6, 1, "")
	writeProtobufField(&field1_21_10, 6, field1_21_10_6.Bytes())
	writeProtobufField(&buf, 10, field1_21_10.Bytes())

	// fields 1.21.11, 1.21.12, 1.21.13, 1.21.14
	for _, f := range []int{11, 12, 13, 14} {
		writeProtobufString(&buf, f, "")
	}

	// field 1.21.19
	var field1_21_19 bytes.Buffer
	writeProtobufString(&field1_21_19, 1, "")
	writeProtobufString(&field1_21_19, 2, "")
	writeProtobufField(&buf, 19, field1_21_19.Bytes())

	return buf.Bytes()
}

// buildAlbumListField1_22 builds field 1.22
func buildAlbumListField1_22() []byte {
	var buf bytes.Buffer
	writeProtobufVarint(&buf, 1, 2)
	return buf.Bytes()
}

// buildAlbumListField1_25 builds field 1.25
func buildAlbumListField1_25() []byte {
	var buf bytes.Buffer

	var field1_25_1 bytes.Buffer
	var field1_25_1_1 bytes.Buffer
	var field1_25_1_1_1 bytes.Buffer
	writeProtobufString(&field1_25_1_1_1, 1, "")
	writeProtobufField(&field1_25_1_1, 1, field1_25_1_1_1.Bytes())
	writeProtobufField(&field1_25_1, 1, field1_25_1_1.Bytes())
	writeProtobufField(&buf, 1, field1_25_1.Bytes())

	writeProtobufString(&buf, 2, "")

	return buf.Bytes()
}

// buildAlbumListRequestField2 builds field 2 of the request
func buildAlbumListRequestField2() []byte {
	var buf bytes.Buffer

	// field 2.1 - nested
	var field2_1 bytes.Buffer
	var field2_1_1 bytes.Buffer
	var field2_1_1_1 bytes.Buffer
	writeProtobufString(&field2_1_1_1, 1, "")
	writeProtobufField(&field2_1_1, 1, field2_1_1_1.Bytes())
	writeProtobufString(&field2_1_1, 2, "")
	writeProtobufField(&field2_1, 1, field2_1_1.Bytes())
	writeProtobufField(&buf, 1, field2_1.Bytes())

	// field 2.2 - empty
	writeProtobufString(&buf, 2, "")

	return buf.Bytes()
}

// parseAlbumListResponse parses the protobuf response and extracts albums
func parseAlbumListResponse(data []byte) (*AlbumListResult, error) {
	result := &AlbumListResult{
		Albums: []AlbumItem{},
	}

	// Parse the response using low-level protobuf parsing
	// The response structure should be similar to media list responses
	// We'll extract albums and pagination token
	albums, paginationToken := extractAlbumsFromResponse(data)

	result.Albums = albums
	result.NextPageToken = paginationToken

	return result, nil
}

// extractAlbumsFromResponse parses the protobuf response bytes and extracts album items
func extractAlbumsFromResponse(data []byte) ([]AlbumItem, string) {
	var albums []AlbumItem
	var paginationToken string

	// Parse the top-level message
	offset := 0
	for offset < len(data) {
		fieldNum, wireType, newOffset := readTag(data, offset)
		if newOffset < 0 {
			break
		}
		offset = newOffset

		switch wireType {
		case 0: // Varint
			_, offset = readVarint(data, offset)
		case 2: // Length-delimited
			length, newOffset := readVarint(data, offset)
			if newOffset < 0 || newOffset+int(length) > len(data) {
				return albums, paginationToken
			}
			fieldData := data[newOffset : newOffset+int(length)]
			offset = newOffset + int(length)

			// Field 1 contains the main response data
			if fieldNum == 1 {
				extractedAlbums, token := parseAlbumResponseField1(fieldData)
				albums = append(albums, extractedAlbums...)
				if token != "" {
					paginationToken = token
				}
			}
		case 5: // 32-bit
			offset += 4
		case 1: // 64-bit
			offset += 8
		default:
			return albums, paginationToken
		}
	}

	return albums, paginationToken
}

// parseAlbumResponseField1 parses the field1 of the response which contains album items
func parseAlbumResponseField1(data []byte) ([]AlbumItem, string) {
	var albums []AlbumItem
	var paginationToken string

	offset := 0
	for offset < len(data) {
		fieldNum, wireType, newOffset := readTag(data, offset)
		if newOffset < 0 {
			break
		}
		offset = newOffset

		switch wireType {
		case 0: // Varint
			_, offset = readVarint(data, offset)
		case 2: // Length-delimited
			length, newOffset := readVarint(data, offset)
			if newOffset < 0 || newOffset+int(length) > len(data) {
				return albums, paginationToken
			}
			fieldData := data[newOffset : newOffset+int(length)]
			offset = newOffset + int(length)

			// Field 4 is the pagination token (for next request's field 1.4)
			if fieldNum == 4 {
				paginationToken = string(fieldData)
			}

			// Try to parse as album - albums may be in different fields
			// This is a simplified parser - adjust based on actual response structure
			album := tryParseAlbumItem(fieldData)
			if album != nil && album.AlbumKey != "" {
				albums = append(albums, *album)
			}
		case 5: // 32-bit
			offset += 4
		case 1: // 64-bit
			offset += 8
		default:
			return albums, paginationToken
		}
	}

	return albums, paginationToken
}

// tryParseAlbumItem attempts to parse a protobuf message as an album item
func tryParseAlbumItem(data []byte) *AlbumItem {
	album := &AlbumItem{}
	hasData := false

	offset := 0
	for offset < len(data) {
		fieldNum, wireType, newOffset := readTag(data, offset)
		if newOffset < 0 {
			break
		}
		offset = newOffset

		switch wireType {
		case 0: // Varint
			value, newOffset := readVarint(data, offset)
			if newOffset >= 0 {
				// Field 3 or similar might be media count
				if fieldNum == 3 || fieldNum == 5 {
					album.MediaCount = int(value)
					hasData = true
				}
			}
			offset = newOffset
		case 2: // Length-delimited (string or nested message)
			length, newOffset := readVarint(data, offset)
			if newOffset < 0 || newOffset+int(length) > len(data) {
				break
			}
			fieldData := data[newOffset : newOffset+int(length)]
			offset = newOffset + int(length)

			// Field 1 might be album key
			if fieldNum == 1 && isPrintableString(fieldData) {
				album.AlbumKey = string(fieldData)
				hasData = true
			}
			// Field 2 might be album title
			if fieldNum == 2 && isPrintableString(fieldData) {
				album.Title = string(fieldData)
				hasData = true
			}
		case 5: // 32-bit
			offset += 4
		case 1: // 64-bit
			offset += 8
		}
	}

	if hasData {
		return album
	}
	return nil
}
