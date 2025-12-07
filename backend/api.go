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
	}

	api.userAgent = fmt.Sprintf(
		"com.google.android.apps.photos/%d (Linux; U; Android 9; %s; %s; Build/PQ2A.190205.001; Cronet/127.0.6510.5) (gzip)",
		api.clientVersionCode,
		api.language,
		api.model,
	)

	return api, nil
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

	var qualityVal int64 = 3
	if AppConfig.Saver {
		qualityVal = 1
		a.model = "Pixel 2"
	}

	if AppConfig.UseQuota {
		a.model = "Pixel 8"
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
			Model:             a.model,
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
		"user-Agent":               a.userAgent,
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
	mediaMetadataFields := []int{1, 3, 4, 5, 6, 7, 15, 16, 17, 19, 20, 21, 25, 30, 31, 32, 33, 34, 36, 37, 38, 39, 40, 41}
	field1_1 := buildEmptyNestedMessage(mediaMetadataFields)
	writeProtobufField(&buf, 1, field1_1)

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
	items, _, _ := extractMediaItemsFromResponse(data, 0)

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
			_, offset = readVarint(data, offset)
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
			offset += 4
		case 1: // 64-bit
			offset += 8
		default:
			return result
		}
	}
	return result
}

// tryParseMediaItemWithKey parses a message that might contain a media item with the target key
func tryParseMediaItemWithKey(data []byte, targetMediaKey string) *MediaItem {
	item := &MediaItem{}

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
					if nested != nil && nested.MediaKey != "" {
						// Only update MediaKey if it matches target or we don't have one yet
						if item.MediaKey == "" {
							item.MediaKey = nested.MediaKey
						}
						// Always update filename and media type if available
						if nested.Filename != "" && item.Filename == "" {
							item.Filename = nested.Filename
						}
						if nested.MediaType != "" && item.MediaType == "" {
							item.MediaType = nested.MediaType
						}
					}
				}
			case 2:
				// Field 2 contains nested metadata with filename at sub-field 4
				filename := extractFilenameFromField2(fieldData)
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
			}
		case 5: // 32-bit
			offset += 4
		case 1: // 64-bit
			offset += 8
		default:
			return item
		}
	}

	return item
}

// extractFilenameFromField2 extracts the filename from field 2 of a media item
// Based on the structure: field2 -> field4 = filename
func extractFilenameFromField2(data []byte) string {
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
				return ""
			}
			fieldData := data[newOffset : newOffset+int(length)]
			offset = newOffset + int(length)

			// Field 4 is the filename
			if fieldNum == 4 && isPrintableString(fieldData) {
				return string(fieldData)
			}
		case 5: // 32-bit
			offset += 4
		case 1: // 64-bit
			offset += 8
		default:
			return ""
		}
	}
	return ""
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
}

// MediaListResult contains the result of a media list request
type MediaListResult struct {
	Items         []MediaItem `json:"items"`
	NextPageToken string      `json:"nextPageToken,omitempty"`
	StateToken    string      `json:"stateToken,omitempty"`
}

// minMediaKeyLength is the minimum expected length for a valid media key string
// Google Photos media keys are typically base64-encoded identifiers > 10 chars
const minMediaKeyLength = 10

// GetMediaList retrieves a list of media items from the library
// This uses a simplified request to fetch media items with pagination support
func (a *Api) GetMediaList(pageToken string, limit int) (*MediaListResult, error) {
	// Build the request using raw protobuf wire format
	// The request structure is complex, so we use a helper to build it
	requestData := buildMediaListRequest(pageToken, limit)

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
	result, err := parseMediaListResponse(bodyBytes, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

// buildMediaListRequest creates the protobuf request for fetching media list
// This implements a simplified version of the get_library_page_init request
func buildMediaListRequest(pageToken string, limit int) []byte {
	var buf bytes.Buffer

	// Build field 1 (request data)
	field1 := buildMediaListRequestField1(pageToken, limit)
	writeProtobufField(&buf, 1, field1)

	// Build field 2 (additional options)
	field2 := buildMediaListRequestField2()
	writeProtobufField(&buf, 2, field2)

	return buf.Bytes()
}

func buildMediaListRequestField1(pageToken string, limit int) []byte {
	var buf bytes.Buffer

	// These field numbers correspond to the Google Photos protobuf schema for media list requests
	// They define which metadata fields to include in the response
	// field1.1 - media metadata options (file info, timestamps, etc.)
	mediaMetadataFields := []int{1, 3, 4, 5, 6, 7, 15, 16, 17, 19, 20, 21, 25, 30, 31, 32, 33, 34, 36, 37, 38, 39, 40, 41}
	field1_1 := buildEmptyNestedMessage(mediaMetadataFields)
	writeProtobufField(&buf, 1, field1_1)

	// field1.2 - page size limit (varint)
	if limit > 0 {
		writeProtobufVarint(&buf, 2, int64(limit))
	}

	// field1.3 - album and collection options
	albumOptions := []int{2, 3, 7, 8, 14, 16, 17, 18, 19, 20, 21, 22, 23, 27, 29, 30, 31, 32, 34, 37, 38, 39, 41}
	field1_3 := buildEmptyNestedMessage(albumOptions)
	writeProtobufField(&buf, 3, field1_3)

	// field1.4 - page token (string)
	if pageToken != "" {
		writeProtobufString(&buf, 4, pageToken)
	}

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
func parseMediaListResponse(data []byte, limit int) (*MediaListResult, error) {
	result := &MediaListResult{
		Items: []MediaItem{},
	}

	// Parse the response using low-level protobuf parsing
	// The response has a complex structure, we need to navigate to the media items
	items, nextPageToken, stateToken := extractMediaItemsFromResponse(data, limit)

	result.Items = items
	result.NextPageToken = nextPageToken
	result.StateToken = stateToken

	return result, nil
}

// shouldAddItem checks if we can add more items based on the limit
func shouldAddItem(currentCount, limit int) bool {
	return limit <= 0 || currentCount < limit
}

// extractMediaItemsFromResponse parses the protobuf response bytes and extracts media items
func extractMediaItemsFromResponse(data []byte, limit int) ([]MediaItem, string, string) {
	var items []MediaItem
	var nextPageToken, stateToken string

	// Parse the top-level message
	offset := 0
	for offset < len(data) && shouldAddItem(len(items), limit) {
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
				return items, nextPageToken, stateToken
			}
			fieldData := data[newOffset : newOffset+int(length)]
			offset = newOffset + int(length)

			// Field 1 contains the main response data
			if fieldNum == 1 {
				remainingLimit := 0
				if limit > 0 {
					remainingLimit = limit - len(items)
				}
				extractedItems, token, state := parseResponseField1(fieldData, remainingLimit)
				items = append(items, extractedItems...)
				if token != "" {
					nextPageToken = token
				}
				if state != "" {
					stateToken = state
				}
			}
		case 5: // 32-bit
			offset += 4
		case 1: // 64-bit
			offset += 8
		default:
			return items, nextPageToken, stateToken
		}
	}

	return items, nextPageToken, stateToken
}

// parseResponseField1 parses the field1 of the response which contains media items
func parseResponseField1(data []byte, limit int) ([]MediaItem, string, string) {
	var items []MediaItem
	var nextPageToken, stateToken string

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
				return items, nextPageToken, stateToken
			}
			fieldData := data[newOffset : newOffset+int(length)]
			offset = newOffset + int(length)

			// Field 2 contains media items array (repeated field)
			if fieldNum == 2 {
				item := tryParseMediaItem(fieldData)
				if item != nil && item.MediaKey != "" && shouldAddItem(len(items), limit) {
					items = append(items, *item)
				}
			}
			// Field 6 is the next page token
			if fieldNum == 6 {
				nextPageToken = string(fieldData)
			}
			// Field 5 is the state token
			if fieldNum == 5 {
				stateToken = string(fieldData)
			}
		case 5: // 32-bit
			offset += 4
		case 1: // 64-bit
			offset += 8
		default:
			return items, nextPageToken, stateToken
		}
	}

	return items, nextPageToken, stateToken
}

// tryParseMediaItem attempts to parse a protobuf message as a media item
func tryParseMediaItem(data []byte) *MediaItem {
	item := &MediaItem{}

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

			// Try to extract media key (field 1) and filename (field 2)
			// These are typically strings
			switch fieldNum {
			case 1:
				// Could be media key (string) or nested message
				if isPrintableString(fieldData) && len(fieldData) > minMediaKeyLength {
					item.MediaKey = string(fieldData)
				} else {
					// Try to parse nested message for media info
					nestedItem := tryParseMediaItem(fieldData)
					if nestedItem != nil && nestedItem.MediaKey != "" {
						item.MediaKey = nestedItem.MediaKey
						if nestedItem.Filename != "" {
							item.Filename = nestedItem.Filename
						}
						if nestedItem.MediaType != "" {
							item.MediaType = nestedItem.MediaType
						}
					}
				}
			case 2:
				// Field 2 is a nested message containing metadata including filename at sub-field 4
				// Try to extract filename from nested structure first
				if filename := extractFilenameFromField2(fieldData); filename != "" {
					item.Filename = filename
				} else if isPrintableString(fieldData) {
					// Fallback: Could be filename or dedup key directly
					if item.Filename == "" && strings.Contains(string(fieldData), ".") {
						item.Filename = string(fieldData)
					} else if item.DedupKey == "" {
						item.DedupKey = string(fieldData)
					}
				}
			case 3:
				// SHA1 hash - skip for now
			case 4:
				// Timestamp nested message
				ts := tryParseTimestamp(fieldData)
				if ts > 0 {
					item.Timestamp = ts
				}
			}
		case 5: // 32-bit
			offset += 4
		case 1: // 64-bit
			offset += 8
		default:
			return item
		}
	}

	return item
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
