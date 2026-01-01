from typing import List


class Solution:
    def longestCommonPrefix(self, strs: List[str]) -> str:
        if not strs:
            return ""

        strs.sort()
        first, last = strs[0], strs[-1]
        prefix = ""

        for i in range(len(first)):
            if first[i] != last[i]:
                return prefix

            prefix += first[i]

        return prefix
