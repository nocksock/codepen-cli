package codepen

import (
	"context"
	"net/http"
	"net/url"
)

// Pen returns a Pen's metadata.
func (c *Client) Pen(ctx context.Context, id string) (Pen, error) {
	var pen Pen
	_, err := c.call(ctx, http.MethodGet, "/pens/"+url.PathEscape(id), nil, &pen)
	return pen, err
}
