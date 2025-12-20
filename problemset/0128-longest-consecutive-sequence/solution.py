from typing import List


class Solution:
    def longestConsecutive(self, nums: List[int]) -> int:
        num_set = set(nums)
        longest_len = 0

        for num in num_set:
            if num - 1 not in num_set:
                curr = num
                curr_len = 1

                while curr + 1 in num_set:
                    curr += 1
                    curr_len += 1
                longest_len = max(curr_len, longest_len)

        return longest_len
