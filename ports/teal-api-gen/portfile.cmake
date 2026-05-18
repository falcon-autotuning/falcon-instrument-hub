vcpkg_from_github(
    OUT_SOURCE_PATH SOURCE_PATH
    REPO falcon-autotuning/teal-api-gen
    REF v1.0.0
    SHA512 b2298e4fac03ffdfa1025982f1575ea8f93b5a093f10456c35ca206a84716da7310ee8a55fb6f82ec5da945f156886fc366bb70713e2899178ab1ebb53da509d
)
vcpkg_cmake_configure(
    SOURCE_PATH "${SOURCE_PATH}"
    OPTIONS
        -DCMAKE_BUILD_TYPE=Release
)
vcpkg_cmake_install()
vcpkg_cmake_config_fixup()
file(INSTALL "${SOURCE_PATH}/LICENSE"
     DESTINATION "${CURRENT_PACKAGES_DIR}/share/${PORT}"
     RENAME copyright)
vcpkg_copy_pdbs()
