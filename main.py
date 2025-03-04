import asyncio
import uvloop

from scripts.leetcode import Leetcode


async def run():
    leetcode = Leetcode()
    await leetcode.main()


if __name__ == "__main__":
    uvloop.install()
    asyncio.run(run())
