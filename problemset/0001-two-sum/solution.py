from typing import List


class Solution:
    def twoSum(self, nums: List[int], target: int) -> List[int]:
        map = {}

        for idx, num in enumerate(nums):
            diff = target - num
            if diff in map:
                return [map[diff], idx]

            map[num] = idx
