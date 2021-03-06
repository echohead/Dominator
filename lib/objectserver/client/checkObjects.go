package client

import (
	"github.com/Cloud-Foundations/Dominator/lib/hash"
	"github.com/Cloud-Foundations/Dominator/proto/objectserver"
)

func (objClient *ObjectClient) checkObjects(hashes []hash.Hash) (
	[]uint64, error) {
	var request objectserver.CheckObjectsRequest
	request.Hashes = hashes
	var reply objectserver.CheckObjectsResponse
	client, err := objClient.getClient()
	if err != nil {
		return nil, err
	}
	err = client.RequestReply("ObjectServer.CheckObjects", request, &reply)
	if err != nil {
		return nil, err
	}
	return reply.ObjectSizes, nil
}
