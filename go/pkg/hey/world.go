package hey

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/url"
	"regexp"
	"strings"
)

// WorldService handles HEY World — the blog you write by sending an email.
//
// None of it is JSON: posts are created by emailing world@hey.com, edits redirect, and the
// subscriber list is a CSV stream.
type WorldService struct {
	client *Client
}

// NewWorldService creates a new WorldService.
func NewWorldService(client *Client) *WorldService {
	return &WorldService{client: client}
}

// WorldAddress is the recipient that turns a message into a HEY World post.
const WorldAddress = "world@hey.com"

// worldPostTokenRe pulls the post token out of the location a publish redirects to.
var worldPostTokenRe = regexp.MustCompile(`/world/posts/([0-9a-f]+)`)

// Publish writes a HEY World post by sending a message to world@hey.com, and returns the
// post's token.
func (s *WorldService) Publish(ctx context.Context, subject, content string) (token string, err error) {
	op := OperationInfo{
		Service: "World", Operation: "PublishWorldPost",
		ResourceType: "world_post", IsMutation: true,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		senderID, serr := s.client.DefaultSenderID(ctx)
		if serr != nil {
			return serr
		}

		values := url.Values{}
		values.Set("acting_sender_id", fmt.Sprintf("%d", senderID))
		values.Set("message[subject]", subject)
		values.Set("message[content]", content)
		values.Set("entry[addressed][directly]", WorldAddress)
		values.Set("entry[status]", "active")

		resp, perr := s.client.PostForm(ctx, "/messages", values)
		if perr != nil {
			return perr
		}

		match := worldPostTokenRe.FindStringSubmatch(resp.Location)
		if match == nil {
			return &Error{Code: CodeAPI, Message: fmt.Sprintf("the message was sent but did not become a HEY World post (landed on %q)", resp.Location)}
		}
		token = match[1]
		return nil
	})
	return token, err
}

// Update edits a published post. Empty fields are left alone.
func (s *WorldService) Update(ctx context.Context, token, subject, content string) error {
	op := OperationInfo{
		Service: "World", Operation: "UpdateWorldPost",
		ResourceType: "world_post", IsMutation: true,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		values := url.Values{}
		if subject != "" {
			values.Set("world_post[subject]", subject)
		}
		if content != "" {
			values.Set("world_post[content]", content)
		}

		_, err := s.client.PatchForm(ctx, "/world/posts/"+url.PathEscape(token), values)
		return err
	})
}

// Delete takes a post off HEY World.
func (s *WorldService) Delete(ctx context.Context, token string) error {
	op := OperationInfo{
		Service: "World", Operation: "DeleteWorldPost",
		ResourceType: "world_post", IsMutation: true,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		_, err := s.client.DeleteForm(ctx, "/world/posts/"+url.PathEscape(token))
		return err
	})
}

// ExportSubscribers returns the confirmed subscribers of a HEY World list as CSV, with the
// columns email_address and subscribed_at. The list is named by its author's email address.
func (s *WorldService) ExportSubscribers(ctx context.Context, listEmailAddress string) (result []byte, err error) {
	op := OperationInfo{
		Service: "World", Operation: "ExportWorldSubscribers",
		ResourceType: "world_list", IsMutation: false,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.GetCSV(ctx, fmt.Sprintf("/world/lists/%s/export.csv", url.PathEscape(listEmailAddress)))
		if rerr != nil {
			return rerr
		}
		result = resp.Data
		return nil
	})
	return result, err
}

// ImportSubscribers uploads a CSV of subscribers to a HEY World list.
func (s *WorldService) ImportSubscribers(ctx context.Context, listEmailAddress, filename string, csv []byte) error {
	op := OperationInfo{
		Service: "World", Operation: "ImportWorldSubscribers",
		ResourceType: "world_list", IsMutation: true,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		body, contentType, berr := subscriberImportBody(filename, csv)
		if berr != nil {
			return berr
		}

		path := fmt.Sprintf("/world/lists/%s/imports", url.PathEscape(listEmailAddress))
		_, err := s.client.PostMultipart(ctx, path, contentType, body)
		return err
	})
}

// subscriberImportBody wraps the CSV in the multipart form the import endpoint expects.
func subscriberImportBody(filename string, csv []byte) ([]byte, string, error) {
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)

	if filename == "" {
		filename = "subscribers.csv"
	}
	if !strings.HasSuffix(filename, ".csv") {
		filename += ".csv"
	}

	part, err := writer.CreateFormFile("world_list_import[source]", filename)
	if err != nil {
		return nil, "", err
	}
	if _, err = part.Write(csv); err != nil {
		return nil, "", err
	}
	if err = writer.Close(); err != nil {
		return nil, "", err
	}

	return buffer.Bytes(), writer.FormDataContentType(), nil
}
