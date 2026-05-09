package protocol

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

func BuildDeviceSignature(method, model, requestID, timestamp, uid, uuid string, params map[string]any, secret string) (string, error) {
	headerString := model + requestID + timestamp + uuid
	if strings.TrimSpace(uid) != "" {
		headerString = model + requestID + timestamp + uid + uuid
	}
	base := strings.ToUpper(method) + "&" + headerString + "&" + buildCanonicalParams(method, params)
	return legacySign(base, secret), nil
}

func buildCanonicalParams(method string, params map[string]any) string {
	if len(params) == 0 {
		return ""
	}

	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	pairs := make([]string, 0, len(keys))
	encodeQuery := isQueryMethod(method)
	for _, key := range keys {
		pairs = append(pairs, key+"="+stringifyValue(params[key], encodeQuery))
	}

	return strings.Join(pairs, "&")
}

func stringifyValue(value any, encodeQuery bool) string {
	result := stringifyRawValue(value)
	if encodeQuery {
		return encodeURIComponent(result)
	}
	return result
}

func stringifyRawValue(value any) string {
	switch v := value.(type) {
	case nil:
		return "null"
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case int:
		return strconv.Itoa(v)
	case int8:
		return strconv.FormatInt(int64(v), 10)
	case int16:
		return strconv.FormatInt(int64(v), 10)
	case int32:
		return strconv.FormatInt(int64(v), 10)
	case int64:
		return strconv.FormatInt(v, 10)
	case uint:
		return strconv.FormatUint(uint64(v), 10)
	case uint8:
		return strconv.FormatUint(uint64(v), 10)
	case uint16:
		return strconv.FormatUint(uint64(v), 10)
	case uint32:
		return strconv.FormatUint(uint64(v), 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	case float32:
		return formatFloat(float64(v))
	case float64:
		return formatFloat(v)
	case []any:
		items := make([]string, 0, len(v))
		for _, item := range v {
			items = append(items, stringifyRawValue(item))
		}
		return strings.Join(items, ",")
	default:
		return "[object Object]"
	}
}

func formatFloat(v float64) string {
	if math.Trunc(v) == v {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func isQueryMethod(method string) bool {
	switch strings.ToUpper(method) {
	case "GET", "DELETE":
		return true
	default:
		return false
	}
}

func encodeURIComponent(value string) string {
	if value == "" {
		return ""
	}

	var builder strings.Builder
	for _, b := range []byte(value) {
		switch {
		case b >= 'A' && b <= 'Z':
			builder.WriteByte(b)
		case b >= 'a' && b <= 'z':
			builder.WriteByte(b)
		case b >= '0' && b <= '9':
			builder.WriteByte(b)
		case strings.ContainsRune("-_.!~*'()", rune(b)):
			builder.WriteByte(b)
		default:
			builder.WriteString(fmt.Sprintf("%%%02X", b))
		}
	}
	return builder.String()
}

func legacySign(baseString, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(baseString))
	hexDigest := hex.EncodeToString(mac.Sum(nil))
	return base64.StdEncoding.EncodeToString([]byte(hexDigest))
}
