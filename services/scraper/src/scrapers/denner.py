import logging

from src.models.product import ScrapedProduct
from src.scrapers.base import BaseScraper

logger = logging.getLogger(__name__)


class DennerScraper(BaseScraper):
    store_slug = "denner"
    store_name = "Denner"
    base_url = "https://www.denner.ch"

    def get_category_urls(self) -> list[str]:
        # TODO: Discover category URLs from Denner
        return []

    def scrape_category(self, category_url: str) -> list[ScrapedProduct]:
        # TODO: Implement Denner scraping
        logger.info("Denner scraper not yet implemented")
        return []
