package aec

// seTableSize is the largest FS value the second-extension table covers
// (libaec's SE_TABLE_SIZE). FS values above this are a data error.
const seTableSize = 90

// buildSETable precomputes the inverse of the second-extension triangular
// map. For a decoded FS value m, table[2m] = d0+d1 (the pair sum) and
// table[2m+1] = the row base (sum)*(sum+1)/2. Mirrors libaec create_se_table.
func buildSETable() []int {
	table := make([]int, 2*(seTableSize+1))
	k := 0
	for i := 0; i < 13; i++ {
		ms := k
		for j := 0; j <= i; j++ {
			table[2*k] = i
			table[2*k+1] = ms
			k++
		}
	}
	return table
}
