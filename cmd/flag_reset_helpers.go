package cmd

import (
	"encoding/csv"
	"strings"

	"github.com/spf13/pflag"
)

// resettableSlice preserves the original Value (including StringSliceVar's
// destination) while tracking the first occurrence after a CLI reset. pflag's
// private changed bit survives Flag.Changed=false, so Set would keep appending.
type resettableSlice struct {
	pflag.Value
	pflag.SliceValue
	defaults []string
	changed  bool
}

func (v *resettableSlice) Set(value string) error {
	if v.changed {
		return v.Value.Set(value)
	}
	previous := v.GetSlice()
	_ = v.Replace([]string{})
	if err := v.Value.Set(value); err != nil {
		_ = v.Replace(previous)
		return err
	}
	v.changed = true
	return nil
}

func resetFlagValue(f *pflag.Flag) {
	f.Value = resetValue(f.Value, f.DefValue)
}

func resetValue(value pflag.Value, defValue string) pflag.Value {
	if repeated, ok := value.(*repeatedFlagValue); ok {
		repeated.Value = resetValue(repeated.Value, defValue)
		return repeated
	}
	if value.Type() != "stringSlice" && value.Type() != "stringArray" {
		_ = value.Set(defValue)
		return value
	}
	v, ok := value.(*resettableSlice)
	if !ok {
		defaults := []string{}
		if raw := strings.TrimSuffix(strings.TrimPrefix(defValue, "["), "]"); raw != "" {
			// pflag serializes these defaults as bracketed CSV.
			defaults, _ = csv.NewReader(strings.NewReader(raw)).Read()
		}
		v = &resettableSlice{Value: value, SliceValue: value.(pflag.SliceValue), defaults: defaults}
	}
	_ = v.Replace(append([]string{}, v.defaults...))
	v.changed = false
	return v
}
