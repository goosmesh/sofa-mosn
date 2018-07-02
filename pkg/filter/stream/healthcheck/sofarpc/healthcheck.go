package sofarpc

import (
	"context"
	"gitlab.alipay-inc.com/afe/mosn/pkg/api/v2"
	"gitlab.alipay-inc.com/afe/mosn/pkg/config"
	"gitlab.alipay-inc.com/afe/mosn/pkg/log"
	"gitlab.alipay-inc.com/afe/mosn/pkg/protocol/sofarpc"
	"gitlab.alipay-inc.com/afe/mosn/pkg/protocol/sofarpc/codec"
	"gitlab.alipay-inc.com/afe/mosn/pkg/types"
	"reflect"
	"time"
)

// todo: support cached pass through

// types.StreamEncoderFilter
type healthCheckFilter struct {
	context context.Context

	// config
	passThrough                  bool
	cacheTime                    time.Duration
	clusterMinHealthyPercentages map[string]float32
	// request properties
	intercept      bool
	protocol       byte
	requestId      uint32
	healthCheckReq bool
	// callbacks
	cb types.StreamDecoderFilterCallbacks
}

func NewHealthCheckFilter(context context.Context, config *v2.HealthCheckFilter) *healthCheckFilter {
	return &healthCheckFilter{
		context:                      context,
		passThrough:                  config.PassThrough,
		cacheTime:                    config.CacheTime,
		clusterMinHealthyPercentages: config.ClusterMinHealthyPercentage,
	}
}

func (f *healthCheckFilter) DecodeHeaders(headers map[string]string, endStream bool) types.FilterHeadersStatus {
	if cmdCodeStr, ok := headers[sofarpc.SofaPropertyHeader(sofarpc.HeaderCmdCode)]; ok {
		cmdCode := sofarpc.ConvertPropertyValue(cmdCodeStr, reflect.Int16)

		//sofarpc.HEARTBEAT(0) is equal to sofarpc.TR_HEARTBEAT(0)
		if cmdCode == sofarpc.HEARTBEAT {
			protocolStr := headers[sofarpc.SofaPropertyHeader(sofarpc.HeaderProtocolCode)]
			f.protocol = sofarpc.ConvertPropertyValue(protocolStr, reflect.Uint8).(byte)
			requestIdStr := headers[sofarpc.SofaPropertyHeader(sofarpc.HeaderReqID)]
			f.requestId = sofarpc.ConvertPropertyValue(requestIdStr, reflect.Uint32).(uint32)
			f.healthCheckReq = true
			f.cb.RequestInfo().SetHealthCheck(true)

			if !f.passThrough {
				f.intercept = true
			}

			endStream = true
		}
	}

	if endStream && f.intercept {
		f.handleIntercept()
	}

	if f.intercept {
		return types.FilterHeadersStatusStopIteration
	} else {
		return types.FilterHeadersStatusContinue
	}
}

func (f *healthCheckFilter) DecodeData(buf types.IoBuffer, endStream bool) types.FilterDataStatus {
	if endStream && f.intercept {
		f.handleIntercept()
	}

	if f.intercept {
		return types.FilterDataStatusStopIterationNoBuffer
	} else {
		return types.FilterDataStatusContinue
	}
}

func (f *healthCheckFilter) DecodeTrailers(trailers map[string]string) types.FilterTrailersStatus {
	if f.intercept {
		f.handleIntercept()
	}

	if f.intercept {
		return types.FilterTrailersStatusStopIteration
	} else {
		return types.FilterTrailersStatusContinue
	}
}

func (f *healthCheckFilter) handleIntercept() {
	// todo: cal status based on cluster healthy host stats and f.clusterMinHealthyPercentages

	var resp interface{}

	//TODO add protocl-level interface for heartbeat process, like Protocols.TriggerHeartbeat(protocolCode, requestId)&Protocols.ReplyHeartbeat(protocolCode, requestId)
	switch f.protocol {
	//case f.protocol == sofarpc.PROTOCOL_CODE:
	//resp = codec.NewTrHeartbeatAck( f.requestId)
	case sofarpc.PROTOCOL_CODE_V1, sofarpc.PROTOCOL_CODE_V2:
		//boltv1 and boltv2 use same heartbeat struct as BoltV1
		resp = codec.NewBoltHeartbeatAck(f.requestId)
	default:
		log.ByContext(f.context).Errorf("Unknown protocol code: [%x] while intercept healthcheck.", f.protocol)
		//TODO: set hijack reply - codec error, actually this would happen at codec stage which is before this
	}

	f.cb.EncodeHeaders(resp, true)
}

func (f *healthCheckFilter) SetDecoderFilterCallbacks(cb types.StreamDecoderFilterCallbacks) {
	f.cb = cb
}

func (f *healthCheckFilter) OnDestroy() {}

// ~~ factory
type HealthCheckFilterConfigFactory struct {
	FilterConfig *v2.HealthCheckFilter
}

func (f *HealthCheckFilterConfigFactory) CreateFilterChain(context context.Context, callbacks types.FilterChainFactoryCallbacks) {
	filter := NewHealthCheckFilter(context, f.FilterConfig)
	callbacks.AddStreamDecoderFilter(filter)
}

func CreateHealthCheckFilterFactory(conf map[string]interface{}) (types.StreamFilterChainFactory, error) {
	return &HealthCheckFilterConfigFactory{
		FilterConfig: config.ParseHealthcheckFilter(conf),
	}, nil
}
