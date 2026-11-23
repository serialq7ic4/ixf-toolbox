package servicedesk

import "testing"

func TestParseEditURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    EditTarget
		wantErr bool
	}{
		{name: "valid", raw: "https://servicedesk.iflytek.com/#/order/order-edit/15?service_id=4003", want: EditTarget{BusinessID: 15, ServiceID: 4003}},
		{name: "valid without origin slash", raw: "https://servicedesk.iflytek.com#/order/order-edit/15?service_id=4003", want: EditTarget{BusinessID: 15, ServiceID: 4003}},
		{name: "http scheme", raw: "http://servicedesk.iflytek.com/#/order/order-edit/15?service_id=4003", wantErr: true},
		{name: "alternate host", raw: "https://example.test/#/order/order-edit/15?service_id=4003", wantErr: true},
		{name: "host with port", raw: "https://servicedesk.iflytek.com:443/#/order/order-edit/15?service_id=4003", wantErr: true},
		{name: "host case differs", raw: "https://ServiceDesk.iflytek.com/#/order/order-edit/15?service_id=4003", wantErr: true},
		{name: "userinfo present", raw: "https://user@servicedesk.iflytek.com/#/order/order-edit/15?service_id=4003", wantErr: true},
		{name: "outer path", raw: "https://servicedesk.iflytek.com/app#/order/order-edit/15?service_id=4003", wantErr: true},
		{name: "query only edit path", raw: "https://servicedesk.iflytek.com/?service_id=4003#/order/order-edit/15", wantErr: true},
		{name: "missing service ID", raw: "https://servicedesk.iflytek.com/#/order/order-edit/15", wantErr: true},
		{name: "duplicate service ID", raw: "https://servicedesk.iflytek.com/#/order/order-edit/15?service_id=4003&service_id=4004", wantErr: true},
		{name: "extra query data", raw: "https://servicedesk.iflytek.com/#/order/order-edit/15?service_id=4003&next=other", wantErr: true},
		{name: "malformed business ID", raw: "https://servicedesk.iflytek.com/#/order/order-edit/abc?service_id=4003", wantErr: true},
		{name: "zero business ID", raw: "https://servicedesk.iflytek.com/#/order/order-edit/0?service_id=4003", wantErr: true},
		{name: "malformed service ID", raw: "https://servicedesk.iflytek.com/#/order/order-edit/15?service_id=4x", wantErr: true},
		{name: "zero service ID", raw: "https://servicedesk.iflytek.com/#/order/order-edit/15?service_id=0", wantErr: true},
		{name: "extra path", raw: "https://servicedesk.iflytek.com/#/order/order-edit/15/more?service_id=4003", wantErr: true},
		{name: "detail path", raw: "https://servicedesk.iflytek.com/#/order/order-detail/15?id=3463015", wantErr: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseEditURL(test.raw)
			if (err != nil) != test.wantErr {
				t.Fatalf("ParseEditURL() error = %v, wantErr %v", err, test.wantErr)
			}
			if err == nil && got != test.want {
				t.Fatalf("ParseEditURL() = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestParseDetailURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    DetailTarget
		wantErr bool
	}{
		{name: "valid", raw: "https://servicedesk.iflytek.com/#/order/order-detail/15?id=3463015", want: DetailTarget{BusinessID: 15, TicketID: "3463015"}},
		{name: "http scheme", raw: "http://servicedesk.iflytek.com/#/order/order-detail/15?id=3463015", wantErr: true},
		{name: "alternate host", raw: "https://servicedesk.invalid/#/order/order-detail/15?id=3463015", wantErr: true},
		{name: "missing ticket ID", raw: "https://servicedesk.iflytek.com/#/order/order-detail/15", wantErr: true},
		{name: "malformed ticket ID", raw: "https://servicedesk.iflytek.com/#/order/order-detail/15?id=34x", wantErr: true},
		{name: "zero ticket ID", raw: "https://servicedesk.iflytek.com/#/order/order-detail/15?id=0", wantErr: true},
		{name: "duplicate ticket ID", raw: "https://servicedesk.iflytek.com/#/order/order-detail/15?id=1&id=2", wantErr: true},
		{name: "extra query data", raw: "https://servicedesk.iflytek.com/#/order/order-detail/15?id=3463015&view=full", wantErr: true},
		{name: "malformed business ID", raw: "https://servicedesk.iflytek.com/#/order/order-detail/nope?id=3463015", wantErr: true},
		{name: "extra path", raw: "https://servicedesk.iflytek.com/#/order/order-detail/15/more?id=3463015", wantErr: true},
		{name: "edit path", raw: "https://servicedesk.iflytek.com/#/order/order-edit/15?service_id=4003", wantErr: true},
		{name: "fragment credentials", raw: "https://servicedesk.iflytek.com/#//other.test/order/order-detail/15?id=3463015", wantErr: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseDetailURL(test.raw)
			if (err != nil) != test.wantErr {
				t.Fatalf("ParseDetailURL() error = %v, wantErr %v", err, test.wantErr)
			}
			if err == nil && got != test.want {
				t.Fatalf("ParseDetailURL() = %+v, want %+v", got, test.want)
			}
		})
	}
}
