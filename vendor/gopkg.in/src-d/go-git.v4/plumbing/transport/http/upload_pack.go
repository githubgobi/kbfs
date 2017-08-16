package http

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/format/pktline"
	"gopkg.in/src-d/go-git.v4/plumbing/protocol/packp"
	"gopkg.in/src-d/go-git.v4/plumbing/transport"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/internal/common"
	"gopkg.in/src-d/go-git.v4/utils/ioutil"
)

type upSession struct {
	*session
}

func newUploadPackSession(c *http.Client, ep transport.Endpoint, auth transport.AuthMethod) (transport.UploadPackSession, error) {
	s, err := newSession(c, ep, auth)

	return &upSession{s}, err
}

func (s *upSession) AdvertisedReferences() (*packp.AdvRefs, error) {
	return advertisedReferences(s.session, transport.UploadPackServiceName)
}

func (s *upSession) UploadPack(req *packp.UploadPackRequest) (*packp.UploadPackResponse, error) {
	if req.IsEmpty() {
		return nil, transport.ErrEmptyUploadPackRequest
	}

	if err := req.Validate(); err != nil {
		return nil, err
	}

	url := fmt.Sprintf(
		"%s/%s",
		s.endpoint.String(), transport.UploadPackServiceName,
	)

	content, err := uploadPackRequestToReader(req)
	if err != nil {
		return nil, err
	}

	res, err := s.doRequest(http.MethodPost, url, content)
	if err != nil {
		return nil, err
	}

	r, err := ioutil.NonEmptyReader(res.Body)
	if err != nil {
		if err == ioutil.ErrEmptyReader || err == io.ErrUnexpectedEOF {
			return nil, transport.ErrEmptyUploadPackRequest
		}

		return nil, err
	}

	rc := ioutil.NewReadCloser(r, res.Body)
	return common.DecodeUploadPackResponse(rc, req)
}

// Close does nothing.
func (s *upSession) Close() error {
	return nil
}

func (s *upSession) doRequest(method, url string, content *bytes.Buffer) (*http.Response, error) {
	var body io.Reader
	if content != nil {
		body = content
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, plumbing.NewPermanentError(err)
	}

	applyHeadersToRequest(req, content, s.endpoint.Host(), transport.UploadPackServiceName)
	s.applyAuthToRequest(req)

	res, err := s.client.Do(req)
	if err != nil {
		return nil, plumbing.NewUnexpectedError(err)
	}

	if err := NewErr(res); err != nil {
		_ = res.Body.Close()
		return nil, err
	}

	return res, nil
}

func uploadPackRequestToReader(req *packp.UploadPackRequest) (*bytes.Buffer, error) {
	buf := bytes.NewBuffer(nil)
	e := pktline.NewEncoder(buf)

	if err := req.UploadRequest.Encode(buf); err != nil {
		return nil, fmt.Errorf("sending upload-req message: %s", err)
	}

	if err := req.UploadHaves.Encode(buf, false); err != nil {
		return nil, fmt.Errorf("sending haves message: %s", err)
	}

	if err := e.EncodeString("done\n"); err != nil {
		return nil, err
	}

	return buf, nil
}
