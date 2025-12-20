from collections import defaultdict
from typing import List


class Solution:
    def topKFrequent(self, nums: List[int], k: int) -> List[int]:
        counter = defaultdict(int)
        for num in nums:
            counter[num] += 1

        result = [
            key
            for key, value in sorted(counter.items(), key=lambda x: x[1], reverse=True)[
                :k
            ]
        ]
        return result
