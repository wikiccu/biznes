package httpserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

func jsonStringInput(c *gin.Context, fields map[string]*string) bool {
	if c.Request.URL.RawQuery != "" || c.Request.URL.ForceQuery {
		WriteError(c, http.StatusBadRequest, "invalid_request", "Query parameters are not accepted.")
		return false
	}
	contentTypes := c.Request.Header.Values("Content-Type")
	mediaType, params, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	charset, hasCharset := params["charset"]
	if len(contentTypes) != 1 || err != nil || mediaType != "application/json" ||
		len(params) > 1 || hasCharset && !strings.EqualFold(charset, "utf-8") {
		WriteError(c, http.StatusUnsupportedMediaType, "unsupported_media_type", "Use application/json with UTF-8 encoding.")
		return false
	}
	if len(params) == 1 && !hasCharset {
		WriteError(c, http.StatusUnsupportedMediaType, "unsupported_media_type", "Use application/json with UTF-8 encoding.")
		return false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8*1024)
	body, err := io.ReadAll(c.Request.Body)
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		WriteError(c, http.StatusRequestEntityTooLarge, "payload_too_large", "The request body is too large.")
		return false
	}
	if err != nil || !utf8.Valid(body) || !validUnicodeEscapes(body) || !decodeStringFields(body, fields) {
		WriteError(c, http.StatusBadRequest, "invalid_request", "Provide one JSON object with the supported string fields.")
		return false
	}
	return true
}

// Token decoding enforces exact field names and rejects duplicate keys/nulls.
func decodeStringFields(body []byte, fields map[string]*string) bool {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return false
	}
	seen := make(map[string]bool, len(fields))
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return false
		}
		name, ok := token.(string)
		if !ok || seen[name] || fields[name] == nil {
			return false
		}
		seen[name] = true
		var value *string
		if err := decoder.Decode(&value); err != nil || value == nil {
			return false
		}
		*fields[name] = *value
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
		return false
	}
	var trailing any
	return decoder.Decode(&trailing) == io.EOF
}

// encoding/json replaces lone surrogate escapes; reject them instead of silently changing strings.
func validUnicodeEscapes(body []byte) bool {
	for i := 0; i < len(body); i++ {
		if body[i] != '\\' {
			continue
		}
		i++
		if i >= len(body) || body[i] != 'u' {
			continue
		}
		if i+4 >= len(body) {
			return false
		}
		code, err := strconv.ParseUint(string(body[i+1:i+5]), 16, 16)
		if err != nil {
			return false
		}
		i += 4
		if code >= 0xd800 && code <= 0xdbff {
			if i+6 >= len(body) || body[i+1] != '\\' || body[i+2] != 'u' {
				return false
			}
			low, err := strconv.ParseUint(string(body[i+3:i+7]), 16, 16)
			if err != nil || low < 0xdc00 || low > 0xdfff {
				return false
			}
			i += 6
		} else if code >= 0xdc00 && code <= 0xdfff {
			return false
		}
	}
	return true
}

func queryInput(c *gin.Context, allowed ...string) (url.Values, bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 0)
	if _, err := io.ReadAll(c.Request.Body); err != nil {
		WriteError(c, http.StatusBadRequest, "invalid_request", "This endpoint requires an empty request body.")
		return nil, false
	}
	query, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil || c.Request.URL.ForceQuery {
		WriteError(c, http.StatusBadRequest, "invalid_request", "Provide valid query parameters.")
		return nil, false
	}
	for key, values := range query {
		if !slices.Contains(allowed, key) || len(values) != 1 || values[0] == "" {
			WriteError(c, http.StatusBadRequest, "invalid_request", "Provide one non-empty value per supported query parameter.")
			return nil, false
		}
	}
	return query, true
}

func paginationInput(c *gin.Context) (int, int, bool) {
	query, ok := queryInput(c, "page", "limit")
	if !ok {
		return 0, 0, false
	}
	return paginationValues(c, query)
}

func paginationValues(c *gin.Context, query url.Values) (int, int, bool) {
	page, limit := 1, 20
	for _, field := range []struct {
		name  string
		value *int
	}{{"page", &page}, {"limit", &limit}} {
		values, present := query[field.name]
		if !present {
			continue
		}
		for _, char := range values[0] {
			if char < '0' || char > '9' {
				WriteError(c, http.StatusBadRequest, "invalid_request", "Pagination parameters require ASCII decimal digits.")
				return 0, 0, false
			}
		}
		value, err := strconv.Atoi(values[0])
		if err != nil {
			WriteError(c, http.StatusBadRequest, "invalid_request", "Pagination parameters are too large.")
			return 0, 0, false
		}
		*field.value = value
	}
	var details []ErrorDetail
	if page < 1 || page > 10000 {
		details = append(details, ErrorDetail{Field: "page", Code: "out_of_range"})
	}
	if limit < 1 || limit > 100 {
		details = append(details, ErrorDetail{Field: "limit", Code: "out_of_range"})
	}
	if len(details) != 0 {
		WriteError(c, http.StatusUnprocessableEntity, "validation_failed", "Please correct the highlighted fields.", details...)
		return 0, 0, false
	}
	return page, limit, true
}
