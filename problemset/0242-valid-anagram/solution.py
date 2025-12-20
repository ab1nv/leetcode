class Solution:
    def isAnagram(self, s: str, t: str) -> bool:
        # s is base to check from
        # need to check t is anagram or not
        # loop over s, start deleting one occurence of each char
        # if loop over and t empty: True
        # else: False

        t_arr = [x for x in t]
        for char in s:
            if char not in t_arr:
                return False

            t_arr.remove(char)

        if t_arr == []:
            return True

        return False
