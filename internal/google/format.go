package google

import "forge.harakara.site/littleisland/henji/v2/internal/proto"

func fromProtoMessages(input []proto.Message) []Content {
	result := make([]Content, 0, len(input))
	for _, in := range input {
		switch in.Role {
		case proto.RoleSystem, proto.RoleUser:
			result = append(result, Content{
				Role:  proto.RoleUser,
				Parts: []Part{{Text: in.Content}},
			})
		}
	}
	return result
}
