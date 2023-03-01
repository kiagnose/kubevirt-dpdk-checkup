package executor

type globalStats struct {
	ID      string `json:"id"`
	Jsonrpc string `json:"jsonrpc"`
	Result  struct {
		MActiveFlows            float64 `json:"m_active_flows"`
		MActiveSockets          int64   `json:"m_active_sockets"`
		MBwPerCore              float64 `json:"m_bw_per_core"`
		MCPUUtil                float64 `json:"m_cpu_util"`
		MCPUUtilRaw             float64 `json:"m_cpu_util_raw"`
		MOpenFlows              float64 `json:"m_open_flows"`
		MPlatformFactor         float64 `json:"m_platform_factor"`
		MRxBps                  float64 `json:"m_rx_bps"`
		MRxCorePps              float64 `json:"m_rx_core_pps"`
		MRxCPUUtil              float64 `json:"m_rx_cpu_util"`
		MRxDropBps              float64 `json:"m_rx_drop_bps"`
		MRxPps                  float64 `json:"m_rx_pps"`
		MSocketUtil             float64 `json:"m_socket_util"`
		MTotalAllocError        int64   `json:"m_total_alloc_error"`
		MTotalClients           int64   `json:"m_total_clients"`
		MTotalNatActive         int64   `json:"m_total_nat_active "`
		MTotalNatLearnError     int64   `json:"m_total_nat_learn_error"`
		MTotalNatNoFid          int64   `json:"m_total_nat_no_fid "`
		MTotalNatOpen           int64   `json:"m_total_nat_open   "`
		MTotalNatSynWait        int64   `json:"m_total_nat_syn_wait"`
		MTotalNatTimeOut        int64   `json:"m_total_nat_time_out"`
		MTotalNatTimeOutWaitAck int64   `json:"m_total_nat_time_out_wait_ack"`
		MTotalQueueDrop         int64   `json:"m_total_queue_drop"`
		MTotalQueueFull         int64   `json:"m_total_queue_full"`
		MTotalRxBytes           int64   `json:"m_total_rx_bytes"`
		MTotalRxPkts            int64   `json:"m_total_rx_pkts"`
		MTotalServers           int64   `json:"m_total_servers"`
		MTotalTxBytes           int64   `json:"m_total_tx_bytes"`
		MTotalTxPkts            int64   `json:"m_total_tx_pkts"`
		MTxBps                  float64 `json:"m_tx_bps"`
		MTxCps                  float64 `json:"m_tx_cps"`
		MTxExpectedBps          float64 `json:"m_tx_expected_bps"`
		MTxExpectedCps          float64 `json:"m_tx_expected_cps"`
		MTxExpectedPps          float64 `json:"m_tx_expected_pps"`
		MTxPps                  float64 `json:"m_tx_pps"`
	} `json:"result"`
}

type portStats struct {
	ID      string `json:"id"`
	Jsonrpc string `json:"jsonrpc"`
	Result  struct {
		Ibytes      int64   `json:"ibytes"`
		Ierrors     int64   `json:"ierrors"`
		Ipackets    int64   `json:"ipackets"`
		MCPUUtil    float64 `json:"m_cpu_util"`
		MTotalRxBps float64 `json:"m_total_rx_bps"`
		MTotalRxPps float64 `json:"m_total_rx_pps"`
		MTotalTxBps float64 `json:"m_total_tx_bps"`
		MTotalTxPps float64 `json:"m_total_tx_pps"`
		Obytes      int64   `json:"obytes"`
		Oerrors     int64   `json:"oerrors"`
		Opackets    int64   `json:"opackets"`
	} `json:"result"`
}
