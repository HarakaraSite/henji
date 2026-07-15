package google

import (
	"encoding/base64"

	"forge.harakara.site/littleisland/henji/v2/internal/proto"
)

func fromProtoMessages(input []proto.Message) []Content {
	result := make([]Content, 0, len(input))
	for _, in := range input {
		switch in.Role {
		case proto.RoleSystem:
			result = append(result, Content{
				Role:  proto.RoleUser,
				Parts: []Part{{Text: in.Content}},
			})
		case proto.RoleUser:
			result = append(result, Content{Role: proto.RoleUser, Parts: googleParts(in)})
		}
	}
	return result
}

func googleParts(in proto.Message) []Part {
	if len(in.Parts) == 0 {
		return []Part{{Text: in.Content}}
	}
	result := make([]Part, 0, len(in.Parts))
	for _, part := range in.Parts {
		switch {
		case part.Type == proto.ContentPartText:
			result = append(result, Part{Text: part.Text})
		case part.Image != nil:
			result = append(result, Part{InlineData: &InlineData{
				MIMEType: part.Image.MediaType,
				Data:     base64.StdEncoding.EncodeToString(part.Image.Data),
			}})
		}
	}
	if len(result) == 0 {
		return []Part{{Text: in.Content}}
	}
	return result
}
