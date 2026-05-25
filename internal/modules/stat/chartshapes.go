package stat

// ChartShapeDescriptor is one selectable chart shape for the plugin
// chart run-mode. Name is the value a plugin sets as the chart
// envelope's `type` (consumed by the frontend StatChart dispatcher);
// LabelKey is the i18n key the frontend resolves for the human label
// (raw Name as fallback). Backend owns this catalog so adding a shape
// is one entry here, surfaced to the UI without a frontend edit.
type ChartShapeDescriptor struct {
	Name     string `json:"name"`
	LabelKey string `json:"label_key"`
}

// chartShapes is the ordered catalog. Names match the StatChart
// dispatcher's accepted `type` values (bar / stacked / line /
// scalars); order is the natural reading order for the dropdown.
var chartShapes = []ChartShapeDescriptor{
	{Name: "bar", LabelKey: "workspace.plugins.chart.shape.bar"},
	{Name: "stacked", LabelKey: "workspace.plugins.chart.shape.stacked"},
	{Name: "line", LabelKey: "workspace.plugins.chart.shape.line"},
	{Name: "scalars", LabelKey: "workspace.plugins.chart.shape.scalars"},
}

// ChartShapes returns the chart-shape catalog (a copy, so callers
// can't mutate the package state).
func ChartShapes() []ChartShapeDescriptor {
	out := make([]ChartShapeDescriptor, len(chartShapes))
	copy(out, chartShapes)
	return out
}
