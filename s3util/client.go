// Copyright (C) 2021  Shanhu Tech Inc.
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU Affero General Public License as published by the
// Free Software Foundation, either version 3 of the License, or (at your
// option) any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE.  See the GNU Affero General Public License
// for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package s3util

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"time"

	minio "github.com/minio/minio-go/v7"
	creds "github.com/minio/minio-go/v7/pkg/credentials"
	"shanhu.io/misc/errcode"
	"shanhu.io/misc/httputil"
)

func makeMinioClient(conf *Config, cred *Credential) (*minio.Client, error) {
	opt := &minio.Options{
		Creds:  creds.NewStaticV4(cred.Key, cred.Secret, ""),
		Secure: true,
	}
	return minio.New(conf.Endpoint, opt)
}

func minioError(err error) error {
	if err == nil {
		return nil
	}

	if status := minio.ToErrorResponse(err).StatusCode; status != 0 {
		return httputil.AddErrCode(status, err)
	}
	return err
}

// Client gives a client for accessing an S3-compatible storage service.
type Client struct {
	client   *minio.Client
	config   *Config
	bucket   string
	basePath string
}

// NewClient creates a new client for accessing a storage endpoint with a base
// path prefix.
func NewClient(config *Config, cred *Credential) (*Client, error) {
	client, err := makeMinioClient(config, cred)
	if err != nil {
		return nil, err
	}
	return &Client{
		client:   client,
		config:   config,
		bucket:   config.Bucket,
		basePath: config.BasePath,
	}, nil
}

// BasePath returns the base path of the client.
func (c *Client) BasePath() string { return c.basePath }

// BaseURL returns the S3 URL of the base path.
func (c *Client) BaseURL() *url.URL {
	return &url.URL{
		Scheme: "s3",
		Host:   c.bucket,
		Path:   c.basePath,
	}
}

func (c *Client) path(p string) string {
	if c.basePath == "" {
		return p
	}
	return path.Join(c.basePath, p)
}

// PresignGet presigns a URL for a future GET. The URL expires in d.
func (c *Client) PresignGet(ctx C, p string, exp time.Duration) (
	*url.URL, error,
) {
	var v url.Values
	return c.client.PresignedGetObject(ctx, c.bucket, c.path(p), exp, v)
}

// PresignPut presigns a URL for a future PUT. The URL expires in d.
func (c *Client) PresignPut(ctx C, p string, exp time.Duration) (
	*url.URL, error,
) {
	return c.client.PresignedPutObject(ctx, c.bucket, c.path(p), exp)
}

// GetBytes gets an object.
func (c *Client) GetBytes(ctx C, p string) ([]byte, error) {
	getError := func(err error) error {
		return errcode.Annotatef(minioError(err), "get %q", p)
	}

	p = c.path(p)
	var opts minio.GetObjectOptions
	obj, err := c.client.GetObject(ctx, c.bucket, p, opts)
	if err != nil {
		return nil, getError(err)
	}
	defer obj.Close()
	bs, err := ioutil.ReadAll(obj)
	if err != nil {
		return nil, getError(err)
	}
	return bs, nil
}

// GetJSON gets a JSON object.
func (c *Client) GetJSON(ctx C, p string, v interface{}) error {
	bs, err := c.GetBytes(ctx, p)
	if err != nil {
		return err
	}
	return json.Unmarshal(bs, v)
}

func (c *Client) put(ctx C, p string, r io.Reader, n int64) error {
	p = c.path(p)
	var opts minio.PutObjectOptions
	if _, err := c.client.PutObject(ctx, c.bucket, p, r, n, opts); err != nil {
		return errcode.Annotatef(minioError(err), "put %q", p)
	}
	return nil
}

// PutFile copies a file.
func (c *Client) PutFile(ctx C, p, fp string) error {
	f, err := os.Open(fp)
	if err != nil {
		return err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return err
	}
	return c.put(ctx, p, f, stat.Size())
}

// PutBytes saves an object.
func (c *Client) PutBytes(ctx C, p string, data []byte) error {
	r := bytes.NewReader(data)
	return c.put(ctx, p, r, int64(len(data)))
}

// PutJSON puts a JSON object.
func (c *Client) PutJSON(ctx C, p string, v interface{}) error {
	bs, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.PutBytes(ctx, p, bs)
}

// Delete deletes an object.
func (c *Client) Delete(ctx C, p string) error {
	var opt minio.RemoveObjectOptions
	if err := c.client.RemoveObject(
		ctx, c.bucket, c.path(p), opt,
	); err != nil {
		return errcode.Annotatef(minioError(err), "delete %q", p)
	}
	return nil
}

// Stat returns the object info a particular object.
func (c *Client) Stat(ctx C, p string) (*minio.ObjectInfo, error) {
	var opts minio.StatObjectOptions
	info, err := c.client.StatObject(ctx, c.bucket, p, opts)
	if err != nil {
		return nil, errcode.Annotatef(minioError(err), "stat %q", p)
	}
	return &info, nil
}
