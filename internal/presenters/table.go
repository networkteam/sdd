package presenters

import (
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
)

// ruledTable builds the header-ruled, borderless table every styled sdd report
// uses (`sdd stats`, `sdd index gc`): faint rule under cyan column labels. The
// first leftCols columns are left-aligned text; the rest are right-aligned.
func ruledTable(headers []string, rows [][]string, leftCols int) string {
	return table.New().
		Border(lipgloss.NormalBorder()).
		BorderTop(false).BorderBottom(false).BorderLeft(false).
		BorderRight(false).BorderColumn(false).BorderRow(false).
		BorderHeader(true).
		BorderStyle(clrFaint).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			s := lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1)
			if row == table.HeaderRow {
				s = s.Inherit(clrKey)
			}
			if col >= leftCols {
				s = s.Align(lipgloss.Right)
			}
			return s
		}).
		Render()
}
