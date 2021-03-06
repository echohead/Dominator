package rpcd

import (
	"github.com/Cloud-Foundations/Dominator/lib/srpc"
	"github.com/Cloud-Foundations/Dominator/proto/dominator"
)

func (t *rpcType) EnableUpdates(conn *srpc.Conn,
	request dominator.EnableUpdatesRequest,
	reply *dominator.EnableUpdatesResponse) error {
	if conn.Username() == "" {
		t.logger.Printf("EnableUpdates(%s)\n", request.Reason)
	} else {
		t.logger.Printf("EnableUpdates(%s): by %s\n",
			request.Reason, conn.Username())
	}
	return t.herd.EnableUpdates()
}
