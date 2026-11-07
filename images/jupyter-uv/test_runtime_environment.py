from pathlib import Path
import unittest


class RuntimeEnvironmentTest(unittest.TestCase):
    def test_image_build_prepares_seed_environment_without_overriding_runtime(self):
        dockerfile = Path(__file__).with_name("Dockerfile").read_text()
        self.assertIn(
            "RUN UV_PROJECT_ENVIRONMENT=/opt/uv/jupyter/.venv uv sync --directory /opt/uv/jupyter",
            dockerfile,
        )
        self.assertNotIn("ENV UV_PROJECT_ENVIRONMENT=", dockerfile)

    def test_runtime_sync_seeds_the_persistent_home_environment(self):
        image_dir = Path(__file__).parent
        start = (image_dir / "jupyter-start.sh").read_text()
        reset = (image_dir / "jupyter-reset.sh").read_text()

        for script in (start, reset):
            self.assertIn('export UV_PROJECT_ENVIRONMENT="/home/jovyan/.venv"', script)
            self.assertIn('cp -R /opt/uv/jupyter/.venv "$UV_PROJECT_ENVIRONMENT"', script)
            self.assertIn("uv sync --locked", script)

        self.assertIn('if [ ! -d "$UV_PROJECT_ENVIRONMENT" ]; then', start)
        self.assertIn('if [ -d "$UV_PROJECT_ENVIRONMENT" ]; then', reset)
        self.assertIn("cp -n /opt/uv/jupyter/pyproject.toml /home/jovyan/", start)
        self.assertIn("cp -n /opt/uv/jupyter/uv.lock /home/jovyan/", start)


if __name__ == "__main__":
    unittest.main()
