package notify

import "testing"

func TestNormalizeNumber(t *testing.T) {
	tests := []struct {
		name    string
		phone   string
		want    string
		wantErr bool
	}{
		{name: "celular com DDD", phone: "(11) 98765-4321", want: "5511987654321"},
		{name: "número com país", phone: "+55 11 98765-4321", want: "5511987654321"},
		{name: "fixo com DDD", phone: "1133334444", want: "551133334444"},
		{name: "sem DDD", phone: "987654321", wantErr: true},
		{name: "outro país", phone: "+1 415 555 2671", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeNumber(tt.phone)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NormalizeNumber(%q) error = %v, wantErr %v", tt.phone, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("NormalizeNumber(%q) = %q, want %q", tt.phone, got, tt.want)
			}
		})
	}
}
