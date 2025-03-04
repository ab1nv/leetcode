import os
import re
import aiohttp
from datetime import datetime


class Leetcode:
    """
    A class to fetch and manage Leetcode problem metadata, create organized problem folders,
    update documentation, and maintain a structured problemset.
    """

    def __init__(self):
        """Initialize the Leetcode scraper instance."""
        self.base_directory = "problemset"
        self.root_readme = "README.md"
        self.headers = {
            "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36",
            "Accept": "application/json",
            "Content-Type": "application/json",
            "Origin": "https://leetcode.com",
            "Referer": "https://leetcode.com/",
        }

    async def fetch_problem_data(self, session, question_slug):
        """
        Fetch problem details asynchronously using Leetcode's GraphQL API.
        """
        graphql_url = "https://leetcode.com/graphql"
        query = {
            "query": """
            query getQuestionDetails($titleSlug: String!) {
                question(titleSlug: $titleSlug) {
                    questionFrontendId
                    title
                    difficulty
                    topicTags { name }
                }
            }""",
            "variables": {"titleSlug": question_slug},
        }

        try:
            async with session.post(
                graphql_url, json=query, headers=self.headers
            ) as response:
                if response.status != 200:
                    print(f"⚠️ GraphQL request failed with status {response.status}.")
                    return None

                data = await response.json()
                if "data" not in data or not data["data"]["question"]:
                    print("⚠️ Invalid response from Leetcode API.")
                    return None

                question = data["data"]["question"]
                return {
                    "question_id": question["questionFrontendId"],
                    "question_title": question["title"],
                    "question_slug": question_slug,
                    "difficulty": question["difficulty"],
                    "topic_tags": ", ".join(
                        tag["name"] for tag in question["topicTags"]
                    ),
                }
        except Exception as e:
            print(f"⚠️ Error fetching problem data: {e}")
            return None

    def extract_question_slug(self, url):
        """Extract the question slug from the given Leetcode URL."""
        match = re.search(r"problems/([^/]+)", url)
        return match.group(1) if match else None

    def setup_problem_directory(self, question_id, question_slug, question_title):
        """Create the problem directory and files if they do not exist."""
        folder_name = f"{question_id}.{question_slug}"
        folder_path = os.path.join(self.base_directory, folder_name)
        os.makedirs(folder_path, exist_ok=True)

        readme_path = os.path.join(folder_path, "README.md")
        solution_path = os.path.join(folder_path, f"{question_slug}.py")

        if not os.path.exists(readme_path):
            open(readme_path, "w").close()

        if not os.path.exists(solution_path):
            with open(solution_path, "w") as f:
                f.write(self.generate_solution_metadata(question_title))

    def generate_solution_metadata(self, question_title):
        """Generate metadata for the solution file using the proper title."""
        current_time = datetime.now().strftime("%d/%m/%Y %H:%M:%S IST")
        return f"""# Author: Abhinav Singh <abhinavsingh@duck.com>
# Question: {question_title}
# Solved On: {current_time}
"""

    def update_main_readme(self, question_data, url):
        """Update the main README.md file with a new problem entry."""

        # Ensure README exists with the proper header
        if not os.path.exists(self.root_readme):
            with open(self.root_readme, "w") as f:
                f.write(
                    "# Leetcode\n\n### Leetcode Problem Index\n\n"
                    "| # | Title | Solution | Tags | Difficulty |\n"
                    "|:----:|:--------:|:--------:|:-------:|:----------:|\n"
                )

        with open(self.root_readme, "r") as f:
            lines = [line.strip() for line in f.readlines() if line.strip()]

        folder_name = f"{question_data['question_id']}.{question_data['question_slug']}"
        solution_path = f"problemset/{folder_name}/{question_data['question_slug']}.py"

        new_entry = f"| {question_data['question_id']} | [{question_data['question_title']}]({url}) | [Python]({solution_path}) | {question_data['topic_tags']} | {question_data['difficulty']} |"

        if new_entry not in lines:
            lines.append(new_entry)

        header = lines[:4]  # First 4 lines include title & table headers
        entries = lines[4:]  # Remaining lines are problem entries

        entries = [line for line in entries if line.count("|") >= 5]

        entries.sort(
            key=lambda x: int(x.split("|")[1].strip())
            if x.split("|")[1].strip().isdigit()
            else float("inf")
        )

        with open(self.root_readme, "w") as f:
            f.writelines("\n".join(header + entries) + "\n")

    async def main(self):
        """Main function to orchestrate the process."""
        url = input("Enter Leetcode Question URL: ")
        question_slug = self.extract_question_slug(url)

        if not question_slug:
            print("❌ Invalid Leetcode URL. Exiting...")
            return

        async with aiohttp.ClientSession() as session:
            question_data = await self.fetch_problem_data(session, question_slug)
            if not question_data:
                print("❌ Failed to fetch problem details. Exiting...")
                return

        self.setup_problem_directory(
            question_data["question_id"],
            question_data["question_slug"],
            question_data["question_title"],
        )
        self.update_main_readme(question_data, url)
        print(
            f"✅ Problem '{question_data['question_title']}' setup completed successfully!"
        )
