package file

import (
	"net/url"
	"strings"
)

// s3://bucket/my-dir/backup.db
// oss://bucket/my-dir/backup.db
func ParseBackupURL(backupURL string) (*url.URL, error) {
	uri, err := url.Parse(backupURL)
	if err != nil {
		return nil, err
	}
	uri.Path = strings.TrimPrefix(uri.Path, "/")
	return uri, nil
}
