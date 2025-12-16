package market

// JSONContractUtils 统一JSON契约工具 - P0修复：确保数据结构输出一致性
// 解决null与[]混用的根本性问题

// EnsureSliceNotNil 确保slice不为nil，统一初始化为空数组
func EnsureSliceNotNil[T any](slice []T) []T {
	if slice == nil {
		return make([]T, 0)
	}
	return slice
}

// InitializeSupplyDemandData 初始化供需区数据，确保所有slice字段非nil
func InitializeSupplyDemandData(data *SupplyDemandData) *SupplyDemandData {
	if data == nil {
		return &SupplyDemandData{
			SupplyZones: make([]*SupplyDemandZone, 0),
			DemandZones: make([]*SupplyDemandZone, 0),
			ActiveZones: make([]*SupplyDemandZone, 0),
		}
	}
	
	data.SupplyZones = EnsureSliceNotNil(data.SupplyZones)
	data.DemandZones = EnsureSliceNotNil(data.DemandZones)
	data.ActiveZones = EnsureSliceNotNil(data.ActiveZones)
	
	return data
}

// InitializeFVGData 初始化FVG数据，确保所有slice字段非nil
func InitializeFVGData(data *FVGData) *FVGData {
	if data == nil {
		return &FVGData{
			BullishFVGs: make([]*FairValueGap, 0),
			BearishFVGs: make([]*FairValueGap, 0),
			ActiveFVGs:  make([]*FairValueGap, 0),
		}
	}
	
	data.BullishFVGs = EnsureSliceNotNil(data.BullishFVGs)
	data.BearishFVGs = EnsureSliceNotNil(data.BearishFVGs)
	data.ActiveFVGs = EnsureSliceNotNil(data.ActiveFVGs)
	
	return data
}

// InitializeFibonacciData 初始化斐波纳契数据，确保所有slice字段非nil
func InitializeFibonacciData(data *FibonacciData) *FibonacciData {
	if data == nil {
		return &FibonacciData{
			Retracements: make([]*FibRetracement, 0),
			Extensions:   make([]*FibExtension, 0),
			Clusters:     make([]*FibCluster, 0),
		}
	}
	
	data.Retracements = EnsureSliceNotNil(data.Retracements)
	data.Extensions = EnsureSliceNotNil(data.Extensions)
	data.Clusters = EnsureSliceNotNil(data.Clusters)
	
	return data
}

// InitializeVolumeProfile 初始化成交量分布数据，确保所有slice字段非nil
func InitializeVolumeProfile(data *VolumeProfile) *VolumeProfile {
	if data == nil {
		return &VolumeProfile{
			Levels: make([]*PriceLevel, 0),
		}
	}
	
	data.Levels = EnsureSliceNotNil(data.Levels)
	
	return data
}

// InitializeDowTheoryData 初始化道氏理论数据，确保所有slice字段非nil
func InitializeDowTheoryData(data *DowTheoryData) *DowTheoryData {
	if data == nil {
		return &DowTheoryData{
			SwingPoints: make([]*SwingPoint, 0),
			TrendLines:  make([]*TrendLine, 0),
		}
	}
	
	data.SwingPoints = EnsureSliceNotNil(data.SwingPoints)
	data.TrendLines = EnsureSliceNotNil(data.TrendLines)
	
	return data
}

// InitializeSupportResistanceData 初始化支撑阻力数据，确保所有slice字段非nil
func InitializeSupportResistanceData(data *SupportResistanceData) *SupportResistanceData {
	if data == nil {
		return &SupportResistanceData{
			KeyLevels: make([]*SRLevel, 0),
			SRFlips:   make([]*SRFlip, 0),
		}
	}
	
	data.KeyLevels = EnsureSliceNotNil(data.KeyLevels)
	data.SRFlips = EnsureSliceNotNil(data.SRFlips)
	
	return data
}

// InitializeChannelData 初始化通道分析数据，确保所有slice字段非nil
func InitializeChannelData(data *ChannelData) *ChannelData {
	if data == nil {
		return &ChannelData{
			TrendLines: make([]*TrendLine, 0),
		}
	}
	
	data.TrendLines = EnsureSliceNotNil(data.TrendLines)
	
	return data
}

// InitializeGate2Data 初始化Gate2数据，确保所有slice字段非nil
func InitializeGate2Data(data *StructureGate2) *StructureGate2 {
	if data == nil {
		return &StructureGate2{
			TopAnchorsLong:  make([]AnchorCandidate, 0),
			TopAnchorsShort: make([]AnchorCandidate, 0),
		}
	}
	
	data.TopAnchorsLong = EnsureSliceNotNil(data.TopAnchorsLong)
	data.TopAnchorsShort = EnsureSliceNotNil(data.TopAnchorsShort)
	
	return data
}