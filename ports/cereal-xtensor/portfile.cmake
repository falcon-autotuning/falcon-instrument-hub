
vcpkg_from_github(
    OUT_SOURCE_PATH SOURCE_PATH
    REPO falcon-autotuning/cereal-xtensor
    REF v${VERSION}
    SHA512 28856ce19074b9dd5156c51bd34d56574997f6b780fd64aa7a53b073bdb90b4077d571bc8c8436d13f77af4898d249a8c735cbad31080aca35a18b6da2ef888a)

vcpkg_cmake_configure(
    SOURCE_PATH "${SOURCE_PATH}"
)

vcpkg_cmake_install()
vcpkg_cmake_config_fixup()
file(INSTALL "${SOURCE_PATH}/LICENSE"
     DESTINATION "${CURRENT_PACKAGES_DIR}/share/${PORT}"
     RENAME copyright)
vcpkg_copy_pdbs()
