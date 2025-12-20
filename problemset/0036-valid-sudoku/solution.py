from typing import List


class Solution:
    def isValidSudoku(self, board: List[List[str]]) -> bool:
        rows = [set() for _ in range(9)]
        columns = [set() for _ in range(9)]
        grids = [set() for _ in range(9)]

        for r in range(9):
            for c in range(9):
                val = board[r][c]

                if val == ".":
                    continue

                if val in rows[r]:
                    return False
                rows[r].add(val)

                if val in columns[c]:
                    return False
                columns[c].add(val)

                grid = (r // 3) * 3 + (c // 3)
                if val in grids[grid]:
                    return False
                grids[grid].add(val)

        return True
