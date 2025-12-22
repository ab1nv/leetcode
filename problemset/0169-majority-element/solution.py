from typing import List


class Solution:
    def majorityElement(self, nums: List[int]) -> int:
        res = 0
        counter = 0

        for i in nums:
            if counter == 0:
                res = i

            if i == res:
                counter += 1
            else:
                counter -= 1

        return res
