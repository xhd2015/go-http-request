package http_request

import (
	"context"
	"errors"
)

var ErrRedirect = errors.New("redirect")

type PlainHtml string

func (c *RequestBuilder) PostJSON(ctx context.Context, url string, data interface{}, res interface{}) error {
	body, err := c.request(ctx, url, true, data, res != nil)
	if err != nil {
		return err
	}
	if res == nil || len(body) == 0 {
		return nil
	}
	if p, ok := res.(*PlainHtml); ok {
		*p = PlainHtml(body)
		return nil
	}
	return unmarshalSafeNumber(body, res)
}

func (c *RequestBuilder) Get(ctx context.Context, url string, res interface{}) error {
	body, err := c.request(ctx, url, false, nil, res != nil)
	if err != nil {
		return err
	}
	if res == nil || len(body) == 0 {
		return nil
	}
	if p, ok := res.(*PlainHtml); ok {
		*p = PlainHtml(body)
		return nil
	}
	return unmarshalSafeNumber(body, res)
}
