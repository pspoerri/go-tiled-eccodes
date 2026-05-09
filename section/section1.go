package section

import (
	"time"

	"github.com/pspoerri/go-tiled-eccodes/internal/bswap"
)

// Section1 — Identification.
//
//	octets 1-4    section length
//	octet  5      section number (=1)
//	octets 6-7    originating centre
//	octets 8-9    originating sub-centre
//	octet  10     master tables version
//	octet  11     local tables version
//	octet  12     significance of reference time
//	octets 13-14  year
//	octet  15     month
//	octet  16     day
//	octet  17     hour
//	octet  18     minute
//	octet  19     second
//	octet  20     production status
//	octet  21     type of processed data
type Section1 struct {
	Raw []byte
}

func (s Section1) Centre() uint16               { return bswap.U16(s.Raw, 5) }
func (s Section1) SubCentre() uint16            { return bswap.U16(s.Raw, 7) }
func (s Section1) MasterTablesVersion() uint8   { return s.Raw[9] }
func (s Section1) LocalTablesVersion() uint8    { return s.Raw[10] }
func (s Section1) SignificanceOfRefTime() uint8 { return s.Raw[11] }
func (s Section1) ProductionStatus() uint8      { return s.Raw[19] }
func (s Section1) TypeOfProcessedData() uint8   { return s.Raw[20] }

func (s Section1) ReferenceTime() time.Time {
	year := int(bswap.U16(s.Raw, 12))
	month := int(s.Raw[14])
	day := int(s.Raw[15])
	hour := int(s.Raw[16])
	minute := int(s.Raw[17])
	second := int(s.Raw[18])
	return time.Date(year, time.Month(month), day, hour, minute, second, 0, time.UTC)
}
