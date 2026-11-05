from pathlib import Path
import unittest


class RuntimeEnvironmentTest(unittest.TestCase):
    def test_runtime_sync_uses_image_environment(self):
        dockerfile = Path(__file__).with_name("Dockerfile").read_text()
        self.assertIn(
            "ENV UV_PROJECT_ENVIRONMENT=/opt/uv/jupyter/.venv",
            dockerfile,
            "runtime uv sync must use the environment built into the image",
        )


if __name__ == "__main__":
    unittest.main()
