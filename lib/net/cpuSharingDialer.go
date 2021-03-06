package net

import (
	"net"
)

type cpuSharingConnection struct {
	net.Conn
	cpuSharer CpuSharer
}

type cpuSharingDialer struct {
	dialer    Dialer
	cpuSharer CpuSharer
}

func newCpuSharingDialer(dialer Dialer, cpuSharer CpuSharer) Dialer {
	return &cpuSharingDialer{dialer: dialer, cpuSharer: cpuSharer}
}

func (d *cpuSharingDialer) Dial(network, address string) (net.Conn, error) {
	d.cpuSharer.ReleaseCpu()
	defer d.cpuSharer.GrabCpu()
	netConn, err := d.dialer.Dial(network, address)
	if err != nil {
		return nil, err
	}
	return &cpuSharingConnection{Conn: netConn, cpuSharer: d.cpuSharer}, nil
}

func (conn *cpuSharingConnection) Read(b []byte) (n int, err error) {
	conn.cpuSharer.ReleaseCpu()
	defer conn.cpuSharer.GrabCpu()
	return conn.Conn.Read(b)
}

func (conn *cpuSharingConnection) Write(b []byte) (n int, err error) {
	conn.cpuSharer.ReleaseCpu()
	defer conn.cpuSharer.GrabCpu()
	return conn.Conn.Write(b)
}
