package cbi_plugin_v1

//go:generate protoc -I=. -I=../../../../../../ --gogo_out=plugins=grpc:. plugin.proto
