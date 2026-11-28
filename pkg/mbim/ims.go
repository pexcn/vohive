package mbim

import (
	"context"
	"encoding/binary"
	"fmt"
	"net/netip"
)

const (
	CIDDeviceSlotMappings   uint32 = 7
	CIDMSBasicConnectExtPCO uint32 = 9
)

// QuerySlotMappings returns zero-based physical slots indexed by the
// logical executor number. An unsupported query supplies no mapping.
func QuerySlotMappings(ctx context.Context, d *Device) ([]uint32, error) {
	resp, err := d.Command(ctx, UUIDMSBasicConnectExtensions, CIDDeviceSlotMappings, CommandTypeQuery, nil)
	if err != nil {
		return nil, err
	}
	if resp.Status != 0 {
		return nil, &StatusError{Op: "DEVICE_SLOT_MAPPINGS", Status: resp.Status}
	}
	info := resp.InfoBuffer
	if len(info) < 4 {
		return nil, fmt.Errorf("MBIM slot mappings are truncated")
	}
	count := int(le.Uint32(info))
	if count < 1 || count > 16 || len(info) < 4+count*8 {
		return nil, fmt.Errorf("invalid MBIM slot mapping count")
	}
	slots := make([]uint32, 0, count)
	for i := range count {
		ref := info[4+i*8:]
		offset, size := uint64(le.Uint32(ref)), uint64(le.Uint32(ref[4:]))
		if size != 4 || offset < uint64(4+count*8) || offset+size > uint64(len(info)) {
			return nil, fmt.Errorf("MBIM slot mapping points outside the response")
		}
		slots = append(slots, le.Uint32(info[offset:]))
	}
	return slots, nil
}

func QuerySessionPCSCF(ctx context.Context, d *Device, session uint32) ([]netip.Addr, error) {
	query := make([]byte, 12)
	le.PutUint32(query, session)
	resp, err := d.Command(ctx, UUIDMSBasicConnectExtensions, CIDMSBasicConnectExtPCO, CommandTypeQuery, query)
	if err != nil {
		return nil, err
	}
	if resp.Status != 0 {
		return nil, &StatusError{Op: "PCO", Status: resp.Status}
	}
	info := resp.InfoBuffer
	if len(info) < 12 || le.Uint32(info) != session || uint64(le.Uint32(info[4:])) > uint64(len(info)-12) {
		return nil, fmt.Errorf("invalid MBIM PCO response")
	}
	return parsePCSCFOptions(info[12 : 12+int(le.Uint32(info[4:]))])
}

func parsePCSCFOptions(pco []byte) ([]netip.Addr, error) {
	if len(pco) == 0 {
		return nil, nil
	}
	if pco[0]&0x80 == 0 {
		return nil, fmt.Errorf("invalid PCO protocol header")
	}
	var servers []netip.Addr
	for pco = pco[1:]; len(pco) != 0; {
		if len(pco) < 3 || len(pco)-3 < int(pco[2]) {
			return nil, fmt.Errorf("truncated PCO container")
		}
		id, size := binary.BigEndian.Uint16(pco), int(pco[2])
		if (id == 0x0001 && size == 16) || (id == 0x000c && size == 4) {
			if addr, ok := netip.AddrFromSlice(pco[3 : 3+size]); ok && addr.IsGlobalUnicast() {
				servers = append(servers, addr.Unmap())
			}
		}
		pco = pco[3+size:]
	}
	return servers, nil
}

const CIDMSBasicConnectExtBaseStationsInfo = 11

type ServingLTECell struct {
	PLMN                string
	CellID, EARFCN, TAC uint32
}

func QueryServingLTECell(ctx context.Context, d *Device) (ServingLTECell, error) {
	query := make([]byte, 20) // No neighboring cells are needed.
	resp, err := d.Command(ctx, UUIDMSBasicConnectExtensions, CIDMSBasicConnectExtBaseStationsInfo, CommandTypeQuery, query)
	if err != nil {
		return ServingLTECell{}, err
	}
	if resp.Status != 0 {
		return ServingLTECell{}, &StatusError{Op: "BASE_STATIONS_INFO", Status: resp.Status}
	}
	data := resp.InfoBuffer
	const ltePair = 28
	if len(data) < ltePair+8 {
		return ServingLTECell{}, fmt.Errorf("MBIM LTE cell reference is missing")
	}
	offset, size := uint64(le.Uint32(data[ltePair:])), uint64(le.Uint32(data[ltePair+4:]))
	if offset < ltePair+8 || size < 36 || offset+size > uint64(len(data)) {
		return ServingLTECell{}, fmt.Errorf("MBIM LTE serving cell unavailable")
	}
	cell := data[offset : offset+size]
	plmn, err := newInfoReader(cell).stringAt(0)
	if err != nil {
		return ServingLTECell{}, err
	}
	return ServingLTECell{PLMN: plmn, CellID: le.Uint32(cell[8:]), EARFCN: le.Uint32(cell[12:]), TAC: le.Uint32(cell[20:])}, nil
}
